package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// RunAgentLoop 使用本轮消息调用 Model，并返回本轮新增的消息。
func RunAgentLoop(
	ctx context.Context,
	prompts []AgentMessage,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) (messages []AgentMessage, runError error) {
	newMessages := slices.Clone(prompts)
	state := loopState{
		current:  initial.WithMessages(prompts...),
		messages: newMessages,
	}
	return runLoopLifecycle(ctx, prompts, config, sink, &state)
}

func runLoopLifecycle(
	ctx context.Context,
	prompts []AgentMessage,
	config LoopConfig,
	sink AgentEventSink,
	state *loopState,
) ([]AgentMessage, error) {
	events := newAgentEventEmitter(sink)
	if err := events.emit(AgentStartEvent{}); err != nil {
		return state.messages, err
	}

	runError := runAgentTurns(ctx, prompts, config, events, state)
	endError := events.emit(AgentEndEvent{Messages: state.messages})

	return state.messages, errors.Join(runError, endError)
}

// toolCallsFrom 提取 AssistantMessage 中的全部工具调用。
func toolCallsFrom(message AssistantMessage) []ToolCall {
	var calls []ToolCall

	for _, content := range message.Content {
		switch value := content.(type) {
		case ToolCall:
			calls = append(calls, value)
		case *ToolCall:
			if value != nil {
				calls = append(calls, *value)
			}
		}
	}

	return calls
}

// newTruncatedToolResultMessage 为参数可能被截断的工具调用创建错误结果。
func newTruncatedToolResultMessage(
	call ToolCall,
	timestamp int64,
) ToolResultMessage {
	message := fmt.Sprintf(
		`Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.`,
		call.Name,
	)

	return newToolResultMessage(
		call,
		newErrorToolResult(message),
		true,
		timestamp,
	)
}
