package agent

import (
	"context"
	"errors"
	"fmt"
)

// ModelID 是 Core 用来标识当前模型的稳定轻量标识。
type ModelID string

// UnknownModelID 表示尚未绑定具体模型。
const UnknownModelID ModelID = "unknown"

// ThinkingLevel 表示请求模型采用的推理强度。
type ThinkingLevel string

const (
	// ThinkingLevelOff 关闭额外推理。
	ThinkingLevelOff ThinkingLevel = "off"
	// ThinkingLevelMinimal 请求最少量推理。
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	// ThinkingLevelLow 请求低强度推理。
	ThinkingLevelLow ThinkingLevel = "low"
	// ThinkingLevelMedium 请求中等强度推理。
	ThinkingLevelMedium ThinkingLevel = "medium"
	// ThinkingLevelHigh 请求高强度推理。
	ThinkingLevelHigh ThinkingLevel = "high"
	// ThinkingLevelXHigh 请求超高强度推理。
	ThinkingLevelXHigh ThinkingLevel = "xhigh"
	// ThinkingLevelMax 请求当前实现允许的最大推理强度。
	ThinkingLevelMax ThinkingLevel = "max"
)

// AgentState 是高层 Agent 当前状态的只读快照。
type AgentState struct {
	SystemPrompt     string
	Model            ModelID
	ThinkingLevel    ThinkingLevel
	Tools            []Tool
	Messages         []AgentMessage
	IsStreaming      bool
	StreamingMessage AgentMessage
	PendingToolCalls map[string]struct{}
	ErrorMessage     string
}

// AgentInitialState 是构造时允许设置的稳定状态。
type AgentInitialState struct {
	SystemPrompt  string
	Model         ModelID
	ThinkingLevel ThinkingLevel
	Tools         []Tool
	Messages      []AgentMessage
}

// AgentOptions 配置高层 Agent。
type AgentOptions struct {
	InitialState *AgentInitialState
	LoopConfig   LoopConfig
	SteeringMode QueueMode
	FollowUpMode QueueMode
}

func defaultAgentState() AgentState {
	return AgentState{
		Model:            UnknownModelID,
		ThinkingLevel:    ThinkingLevelOff,
		Tools:            []Tool{},
		Messages:         []AgentMessage{},
		PendingToolCalls: map[string]struct{}{},
	}
}

func initialAgentState(initial *AgentInitialState) (AgentState, error) {
	if initial == nil {
		return defaultAgentState(), nil
	}
	state := defaultAgentState()
	state.SystemPrompt = initial.SystemPrompt
	state.Model = initial.Model
	state.ThinkingLevel = initial.ThinkingLevel
	state.Tools = append([]Tool(nil), initial.Tools...)
	messages, err := cloneAgentMessages(initial.Messages)
	if err != nil {
		return AgentState{}, fmt.Errorf("clone initial agent state: %w", err)
	}
	state.Messages = messages
	if state.Model == "" {
		state.Model = UnknownModelID
	}
	if state.ThinkingLevel == "" {
		state.ThinkingLevel = ThinkingLevelOff
	}
	return state, nil
}

func cloneAgentState(state AgentState) (AgentState, error) {
	cloned := state
	cloned.Tools = append([]Tool(nil), state.Tools...)
	if state.Messages != nil {
		messages, err := cloneAgentMessages(state.Messages)
		if err != nil {
			return AgentState{}, fmt.Errorf("clone state messages: %w", err)
		}
		cloned.Messages = messages
	}
	if state.StreamingMessage != nil {
		streaming, cloneError := cloneAgentMessages([]AgentMessage{state.StreamingMessage})
		if cloneError != nil {
			return AgentState{}, fmt.Errorf("clone streaming message: %w", cloneError)
		}
		cloned.StreamingMessage = streaming[0]
	}
	cloned.PendingToolCalls = clonePendingToolCalls(state.PendingToolCalls)
	return cloned, nil
}

func clonePendingToolCalls(calls map[string]struct{}) map[string]struct{} {
	if calls == nil {
		return nil
	}
	cloned := make(map[string]struct{}, len(calls))
	for callID := range calls {
		cloned[callID] = struct{}{}
	}
	return cloned
}

// Abort 幂等取消当前 run；idle 时不执行任何操作。
func (agent *Agent) Abort() error {
	_, err := agent.execute(agentStateCommand{operation: agentStateAbort})
	return err
}

// Reset 在 idle 时清空 transcript 和运行态，并保留 Agent 配置。
func (agent *Agent) Reset() error {
	_, err := agent.execute(agentStateCommand{operation: agentStateReset})
	return err
}

// Prompt 启动一次低层 Agent Loop，并等待 subscriber 全部完成。
func (agent *Agent) Prompt(ctx context.Context, prompts ...AgentMessage) error {
	return agent.executeRun(ctx, func(
		runContext context.Context,
		state AgentState,
		sink AgentEventSink,
	) error {
		clonedPrompts, err := cloneAgentMessages(prompts)
		if err != nil {
			return fmt.Errorf("clone agent prompts: %w", err)
		}
		_, err = RunAgentLoop(
			runContext,
			clonedPrompts,
			agentContextFromState(state),
			agent.loopConfigForRun(false),
			sink,
		)
		return err
	})
}

// Continue 从当前 transcript 尾部继续低层 Agent Loop。
func (agent *Agent) Continue(ctx context.Context) error {
	return agent.executeRun(ctx, func(
		runContext context.Context,
		state AgentState,
		sink AgentEventSink,
	) error {
		return agent.continueFromState(runContext, state, sink)
	})
}

func (agent *Agent) continueFromState(
	ctx context.Context,
	state AgentState,
	sink AgentEventSink,
) error {
	if len(state.Messages) > 0 {
		last := state.Messages[len(state.Messages)-1]
		switch last.(type) {
		case AssistantMessage, *AssistantMessage:
			return agent.continueFromAssistant(ctx, state, sink)
		}
	}
	_, err := ContinueAgentLoop(
		ctx,
		agentContextFromState(state),
		agent.loopConfigForRun(false),
		sink,
	)
	return err
}

func (agent *Agent) continueFromAssistant(
	ctx context.Context,
	state AgentState,
	sink AgentEventSink,
) error {
	steering, err := agent.drainQueue(agentSteeringQueue)
	if err != nil {
		return err
	}
	if len(steering) > 0 {
		return agent.runQueuedPrompt(ctx, state, steering, true, sink)
	}
	followUp, err := agent.drainQueue(agentFollowUpQueue)
	if err != nil {
		return err
	}
	if len(followUp) > 0 {
		return agent.runQueuedPrompt(ctx, state, followUp, false, sink)
	}
	return &ContinuationError{Cause: ErrAssistantContinuation}
}

func (agent *Agent) runQueuedPrompt(
	ctx context.Context,
	state AgentState,
	prompts []AgentMessage,
	skipInitialSteering bool,
	sink AgentEventSink,
) error {
	_, err := RunAgentLoop(
		ctx,
		prompts,
		agentContextFromState(state),
		agent.loopConfigForRun(skipInitialSteering),
		sink,
	)
	return err
}

type agentRunExecutor func(context.Context, AgentState, AgentEventSink) error

func (agent *Agent) executeRun(
	ctx context.Context,
	executor agentRunExecutor,
) (result error) {
	runContext, cancel := context.WithCancel(ctx)
	reply, err := agent.execute(agentStateCommand{
		operation: agentStateBeginRun,
		runDone:   make(chan struct{}),
		cancel:    cancel,
	})
	if err != nil {
		cancel()
		return err
	}
	subscriberErrors := make([]error, 0)
	dispatcher := agentRunEventDispatcher{
		agent: agent, ctx: runContext, failures: &subscriberErrors,
	}
	defer func() {
		_, finishError := agent.execute(agentStateCommand{operation: agentStateFinishRun})
		cancel()
		result = errors.Join(result, errors.Join(subscriberErrors...), finishError)
	}()
	runError := executor(runContext, reply.state, dispatcher.handle)
	lifecycleError := dispatcher.complete(runError, reply.state.Model)
	return errors.Join(runError, lifecycleError)
}

func agentContextFromState(state AgentState) AgentContext {
	return AgentContext{
		SystemPrompt: state.SystemPrompt,
		Messages:     state.Messages,
		Tools:        state.Tools,
	}
}

// WaitForIdle 等待调用时的 run settled；关闭发起后则等待 owner 完全停止。
func (agent *Agent) WaitForIdle(ctx context.Context) error {
	if agent.closing.Load() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for agent close: %w", ctx.Err())
		case <-agent.done:
			return nil
		}
	}
	reply, err := agent.execute(agentStateCommand{operation: agentStateCurrentIdle})
	if err != nil {
		return err
	}
	if reply.idle == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for agent idle: %w", ctx.Err())
	case <-reply.idle:
		return nil
	}
}
