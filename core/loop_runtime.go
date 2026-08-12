package agent

import (
	"context"
	"errors"
	"fmt"
)

// TransformContextFunc 在转换为 Model 消息之前调整 Agent 消息快照。
type TransformContextFunc func(context.Context, []AgentMessage) ([]AgentMessage, error)

// ShouldStopAfterTurnContext 是一轮完整结束后的只读快照。
type ShouldStopAfterTurnContext struct {
	Message     AssistantMessage
	ToolResults []ToolResultMessage
	Context     AgentContext
	NewMessages []AgentMessage
}

// PrepareNextTurnContext 是 PrepareNextTurn 接收的轮次快照。
type PrepareNextTurnContext = ShouldStopAfterTurnContext

// NextTurnUpdate 描述下一轮需要替换的运行状态。
type NextTurnUpdate struct {
	Context *AgentContext
}

// PrepareNextTurnFunc 在下一轮 Model 调用之前生成运行状态更新。
type PrepareNextTurnFunc func(context.Context, PrepareNextTurnContext) (NextTurnUpdate, error)

// ShouldStopAfterTurnFunc 在当前轮完整闭合后决定是否优雅停止。
type ShouldStopAfterTurnFunc func(context.Context, ShouldStopAfterTurnContext) (bool, error)

// GetQueuedMessagesFunc 返回要在下一轮边界注入的消息。
type GetQueuedMessagesFunc func(context.Context) ([]AgentMessage, error)

func cloneAgentContext(current AgentContext) (AgentContext, error) {
	messages, err := cloneAgentMessages(current.Messages)
	if err != nil {
		return AgentContext{}, err
	}
	cloned := current
	cloned.Messages = messages
	cloned.Tools = append([]Tool(nil), current.Tools...)
	return cloned, nil
}

func prepareNextTurn(
	ctx context.Context,
	hook PrepareNextTurnFunc,
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) error {
	if hook == nil {
		return nil
	}
	snapshot, err := newPrepareNextTurnContext(state, message, toolResults)
	if err != nil {
		return fmt.Errorf("snapshot prepare next turn context: %w", err)
	}
	update, err := hook(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("prepare next turn: %w", err)
	}
	if update.Context == nil {
		return nil
	}
	next, err := cloneAgentContext(*update.Context)
	if err != nil {
		return fmt.Errorf("snapshot next turn update: %w", err)
	}
	state.current = next
	return nil
}

func newPrepareNextTurnContext(
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) (PrepareNextTurnContext, error) {
	cloner := newSnapshotCloner()
	messageSnapshot, err := cloner.cloneAssistantMessage(message)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	resultSnapshots := make([]ToolResultMessage, len(toolResults))
	for index, result := range toolResults {
		resultSnapshots[index], err = cloner.cloneToolResultMessage(result)
		if err != nil {
			return PrepareNextTurnContext{}, err
		}
	}
	contextSnapshot, err := cloneAgentContext(state.current)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	newMessages, err := cloneAgentMessages(state.messages)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	return PrepareNextTurnContext{
		Message: messageSnapshot, ToolResults: resultSnapshots,
		Context: contextSnapshot, NewMessages: newMessages,
	}, nil
}

// ErrTurnControlHook 表示轮次控制 Hook 执行失败。
var ErrTurnControlHook = errors.New("agent: turn control hook failed")

// TurnControlHookError 保留失败 Hook 的名称和原始错误。
type TurnControlHookError struct {
	Hook  string
	Cause error
}

// Error 返回轮次控制 Hook 的错误说明。
func (hookError *TurnControlHookError) Error() string {
	return fmt.Sprintf("agent: turn control hook %s failed: %v", hookError.Hook, hookError.Cause)
}

// Unwrap 同时暴露通用哨兵错误和原始错误。
func (hookError *TurnControlHookError) Unwrap() []error {
	return []error{ErrTurnControlHook, hookError.Cause}
}

func shouldStopAfterTurn(
	ctx context.Context,
	hook ShouldStopAfterTurnFunc,
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) (bool, error) {
	if hook == nil {
		return false, nil
	}
	snapshot, err := newPrepareNextTurnContext(state, message, toolResults)
	if err != nil {
		return false, fmt.Errorf("snapshot should stop after turn context: %w", err)
	}
	stop, err := hook(ctx, snapshot)
	if err != nil {
		return false, &TurnControlHookError{Hook: "ShouldStopAfterTurn", Cause: err}
	}
	return stop, nil
}

func getQueuedMessages(
	ctx context.Context,
	hook GetQueuedMessagesFunc,
	hookName string,
) ([]AgentMessage, error) {
	if hook == nil {
		return nil, nil
	}
	messages, err := hook(ctx)
	if err != nil {
		return nil, &TurnControlHookError{Hook: hookName, Cause: err}
	}
	snapshot, err := cloneAgentMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("snapshot queued messages from %s: %w", hookName, err)
	}
	return snapshot, nil
}

func injectQueuedMessages(
	events agentEventEmitter,
	state *loopState,
	messages []AgentMessage,
) error {
	for _, message := range messages {
		if err := emitMessageLifecycle(events, message); err != nil {
			return err
		}
		state.messages = append(state.messages, message)
		state.current = state.current.WithMessages(message)
	}
	return nil
}

type loopState struct {
	current  AgentContext
	messages []AgentMessage
}

type completedTurn struct {
	response    AssistantMessage
	toolResults []ToolResultMessage
	continues   bool
}

func runAgentTurns(
	ctx context.Context,
	prompts []AgentMessage,
	config LoopConfig,
	events agentEventEmitter,
	state *loopState,
) error {
	runtime, err := config.runtime()
	if err != nil {
		return fmt.Errorf("run agent loop: %w", err)
	}
	pending, err := getQueuedMessages(ctx, runtime.getSteeringMessages, "GetSteeringMessages")
	if err != nil {
		return err
	}

	for turn := 0; ; turn++ {
		completed, turnError := runSingleTurn(ctx, turn, prompts, pending, runtime, events, state)
		if turnError != nil {
			return turnError
		}
		if isErrorTerminal(completed.response.StopReason) {
			return nil
		}
		if err := prepareNextTurn(ctx, runtime.prepareNextTurn, state, completed.response, completed.toolResults); err != nil {
			return err
		}
		stop, err := shouldStopAfterTurn(ctx, runtime.shouldStopAfterTurn, state, completed.response, completed.toolResults)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		pending, err = getQueuedMessages(ctx, runtime.getSteeringMessages, "GetSteeringMessages")
		if err != nil {
			return err
		}
		if completed.continues || len(pending) > 0 {
			continue
		}
		pending, err = getQueuedMessages(ctx, runtime.getFollowUpMessages, "GetFollowUpMessages")
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
	}
}

func runSingleTurn(
	ctx context.Context,
	turn int,
	prompts []AgentMessage,
	pending []AgentMessage,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
) (completedTurn, error) {
	if err := validateTurn(ctx, turn, runtime.maxTurns); err != nil {
		return completedTurn{}, err
	}
	if err := events.emit(AgentTurnStartEvent{}); err != nil {
		return completedTurn{}, err
	}
	if turn == 0 {
		if err := emitPromptMessages(events, prompts); err != nil {
			return completedTurn{}, err
		}
	}
	if err := injectQueuedMessages(events, state, pending); err != nil {
		return completedTurn{}, err
	}
	response, err := streamAssistantTurn(ctx, runtime, events, state)
	if err != nil {
		return completedTurn{}, err
	}
	toolResults, continues, err := completeTurn(ctx, runtime, events, state, response)
	if err != nil {
		return completedTurn{}, err
	}
	if err := events.emit(AgentTurnEndEvent{Message: response, ToolResults: toolResults}); err != nil {
		return completedTurn{}, err
	}
	return completedTurn{response: response, toolResults: toolResults, continues: continues}, nil
}

func isErrorTerminal(reason StopReason) bool {
	return reason == StopReasonError || reason == StopReasonAborted
}

func validateTurn(ctx context.Context, turn, maxTurns int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run agent loop: %w", err)
	}
	if turn >= maxTurns {
		return &MaxTurnsError{MaxTurns: maxTurns}
	}
	return nil
}

func emitPromptMessages(events agentEventEmitter, prompts []AgentMessage) error {
	for _, message := range prompts {
		if err := events.emit(AgentMessageStartEvent{Message: message}); err != nil {
			return err
		}
		if err := events.emit(AgentMessageEndEvent{Message: message}); err != nil {
			return err
		}
	}
	return nil
}

func streamAssistantTurn(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
) (AssistantMessage, error) {
	modelContext, err := state.current.toLLM(
		ctx,
		runtime.transformContext,
		runtime.convertToLLM,
	)
	if err != nil {
		return AssistantMessage{}, fmt.Errorf("convert messages to llm: %w", err)
	}
	lifecycle := assistantLifecycle{events: events}
	response, err := runtime.stream(ctx, modelContext, lifecycle.sink())
	if err != nil {
		return AssistantMessage{}, fmt.Errorf("stream assistant response: %w", err)
	}
	if err := lifecycle.finish(response); err != nil {
		return AssistantMessage{}, err
	}
	state.messages = append(state.messages, response)
	state.current = state.current.WithMessages(response)
	return response, nil
}

func completeTurn(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	response AssistantMessage,
) ([]ToolResultMessage, bool, error) {
	calls := toolCallsFrom(response)
	if response.StopReason == StopReasonError || response.StopReason == StopReasonAborted {
		return nil, false, nil
	}
	if len(calls) == 0 {
		return nil, false, nil
	}
	if response.StopReason == StopReasonLength {
		results, err := appendTruncatedResults(runtime, events, state, calls)
		return results, true, err
	}

	results, terminate, err := executeToolCalls(ctx, runtime, events, state, response, calls)
	return results, !terminate, err
}

func appendTruncatedResults(
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	calls []ToolCall,
) ([]ToolResultMessage, error) {
	results := make([]ToolResultMessage, 0, len(calls))
	for _, call := range calls {
		result := newTruncatedToolResultMessage(call, runtime.clock().UnixMilli())
		appendToolResult(state, result)
		if err := emitMessageLifecycle(events, result); err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func executeToolCallsSequential(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	response AssistantMessage,
	calls []ToolCall,
) ([]ToolResultMessage, bool, error) {
	results := make([]ToolResultMessage, 0, len(calls))
	allTerminate := true
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return results, false, fmt.Errorf("execute tool call: %w", err)
		}
		if err := events.emit(AgentToolExecutionStartEvent{
			ToolCallID: call.ID, ToolName: call.Name, Arguments: call.Arguments,
		}); err != nil {
			return results, false, err
		}
		onUpdate := func(partial ToolResult) error {
			return events.emit(AgentToolExecutionUpdateEvent{
				ToolCallID: call.ID, ToolName: call.Name,
				Arguments: call.Arguments, PartialResult: partial,
			})
		}
		outcome, err := executeToolCall(
			ctx,
			call,
			runtime.clock().UnixMilli(),
			onUpdate,
			toolCallExecutionOptions{
				assistantMessage: response,
				context:          state.current,
				validator:        runtime.argumentValidator,
				before:           runtime.beforeToolCall,
				after:            runtime.afterToolCall,
			},
		)
		if err != nil {
			return results, false, err
		}
		appendToolResult(state, outcome.message)
		if err := events.emit(toolExecutionEndEvent(outcome)); err != nil {
			return results, false, err
		}
		if err := emitMessageLifecycle(events, outcome.message); err != nil {
			return results, false, err
		}
		results = append(results, outcome.message)
		allTerminate = allTerminate && outcome.terminate
	}
	return results, allTerminate, nil
}

func appendToolResult(state *loopState, result ToolResultMessage) {
	state.messages = append(state.messages, result)
	state.current = state.current.WithMessages(result)
}

func emitMessageLifecycle(events agentEventEmitter, message AgentMessage) error {
	if err := events.emit(AgentMessageStartEvent{Message: message}); err != nil {
		return err
	}
	return events.emit(AgentMessageEndEvent{Message: message})
}

func toolExecutionEndEvent(outcome toolCallOutcome) AgentToolExecutionEndEvent {
	return AgentToolExecutionEndEvent{
		ToolCallID: outcome.message.ToolCallID,
		ToolName:   outcome.message.ToolName,
		Result:     outcome.result,
		IsError:    outcome.isError,
	}
}
