package agent

import (
	"context"
	"fmt"
)

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
