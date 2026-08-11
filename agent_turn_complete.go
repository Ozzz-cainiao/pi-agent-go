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

	results, err := executeToolCalls(ctx, runtime, events, state, calls)
	return results, true, err
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

func executeToolCalls(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	calls []ToolCall,
) ([]ToolResultMessage, error) {
	results := make([]ToolResultMessage, 0, len(calls))
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("execute tool call: %w", err)
		}
		if err := events.emit(AgentToolExecutionStartEvent{
			ToolCallID: call.ID, ToolName: call.Name, Arguments: call.Arguments,
		}); err != nil {
			return results, err
		}
		result := executeToolCall(ctx, state.current.Tools, call, runtime.clock().UnixMilli())
		appendToolResult(state, result)
		if err := events.emit(toolExecutionEndEvent(result)); err != nil {
			return results, err
		}
		if err := emitMessageLifecycle(events, result); err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
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

func toolExecutionEndEvent(message ToolResultMessage) AgentToolExecutionEndEvent {
	return AgentToolExecutionEndEvent{
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
		Result: ToolResult{
			Content: message.Content, Details: message.Details, Usage: message.Usage,
			AddedToolNames: message.AddedToolNames,
		},
		IsError: message.IsError,
	}
}
