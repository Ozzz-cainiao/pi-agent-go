package agent

import (
	"context"
	"fmt"
	"slices"
)

// RunAgentLoop 使用本轮消息调用 Model，并返回本轮新增的消息。
func RunAgentLoop(
	ctx context.Context,
	prompts []AgentMessage,
	initial AgentContext,
	config LoopConfig,
	emit AssistantMessageEventSink,
) ([]AgentMessage, error) {
	newMessages := slices.Clone(prompts)
	current := initial.WithMessages(prompts...)
	runtime, err := config.runtime()
	if err != nil {
		return newMessages, fmt.Errorf("run agent loop: %w", err)
	}

	emit = assistantMessageEventSinkOrDiscard(emit)

	for turn := 0; ; turn++ {
		if err := ctx.Err(); err != nil {
			return newMessages, fmt.Errorf("run agent loop: %w", err)
		}
		if turn >= runtime.maxTurns {
			return newMessages, &MaxTurnsError{MaxTurns: runtime.maxTurns}
		}

		modelContext, err := current.toLLM(ctx, runtime.convertToLLM)
		if err != nil {
			return newMessages, fmt.Errorf("convert messages to llm: %w", err)
		}

		response, err := runtime.stream(ctx, modelContext, emit)
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
		case StopReasonPending, StopReasonStop, StopReasonToolUse, StopReasonDeferred:
		case StopReasonError, StopReasonAborted:
			return newMessages, nil

		case StopReasonLength:
			if len(toolCalls) == 0 {
				return newMessages, nil
			}

			for _, call := range toolCalls {
				result := newTruncatedToolResultMessage(
					call,
					runtime.clock().UnixMilli(),
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
				runtime.clock().UnixMilli(),
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
