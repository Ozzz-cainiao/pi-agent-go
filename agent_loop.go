package agent

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// RunAgentLoop 使用本轮消息调用 Model，并返回本轮新增的消息。
func RunAgentLoop(
	ctx context.Context,
	prompts []Message,
	initial AgentContext,
	streamFn StreamFunc,
	emit AssistantMessageEventSink,
) ([]Message, error) {
	newMessages := slices.Clone(prompts)
	current := initial.WithMessages(prompts...)

	if emit == nil {
		emit = func(AssistantMessageEvent) error {
			return nil
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return newMessages, fmt.Errorf("run agent loop: %w", err)
		}

		response, err := streamFn(ctx, current, emit)
		if err != nil {
			return newMessages, fmt.Errorf(
				"stream assistant response: %w",
				err,
			)
		}

		newMessages = append(newMessages, response)
		current = current.WithMessages(response)

		toolCalls := toolCallsFrom(response)

		switch response.StopReason {
		case StopReasonError, StopReasonAborted:
			return newMessages, nil

		case StopReasonLength:
			if len(toolCalls) == 0 {
				return newMessages, nil
			}

			for _, call := range toolCalls {
				result := newTruncatedToolResultMessage(
					call,
					time.Now().UnixMilli(),
				)

				newMessages = append(newMessages, result)
				current = current.WithMessages(result)
			}

			continue
		}

		if len(toolCalls) == 0 {
			return newMessages, nil
		}

		for _, call := range toolCalls {
			if err := ctx.Err(); err != nil {
				return newMessages, fmt.Errorf(
					"execute tool call: %w",
					err,
				)
			}

			result := executeToolCall(
				ctx,
				current.Tools,
				call,
				time.Now().UnixMilli(),
			)

			newMessages = append(newMessages, result)
			current = current.WithMessages(result)
		}
	}
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
