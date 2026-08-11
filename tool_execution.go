package agent

import (
	"context"
	"fmt"
)

// executeToolCall 执行一次工具调用，并将结果转换为标准消息。
func executeToolCall(
	ctx context.Context,
	call ToolCall,
	timestamp int64,
	onUpdate toolUpdateSink,
	options toolCallExecutionOptions,
) (toolCallOutcome, error) {
	tool, ok := findToolByName(options.context.Tools, call.Name)
	if !ok {
		result := newErrorToolResult(
			"Tool " + call.Name + " not found",
		)

		return newToolCallOutcome(call, result, true, timestamp), nil
	}
	preparedCall, err := prepareToolCallArguments(tool, call)
	if err != nil {
		result := newErrorToolResult(
			fmt.Sprintf("Tool %q argument preparation failed: %v", call.Name, err),
		)
		return newToolCallOutcome(call, result, true, timestamp), nil
	}
	if err := options.validator.Validate(ctx, tool.Definition(), preparedCall.Arguments); err != nil {
		result := newErrorToolResult(
			fmt.Sprintf("Tool %q arguments invalid: %v", call.Name, err),
		)
		return newToolCallOutcome(call, result, true, timestamp), nil
	}
	beforeResult, err := runBeforeToolCall(
		ctx,
		options.before,
		options,
		call,
		&preparedCall,
	)
	if err != nil {
		result := newErrorToolResult(
			fmt.Sprintf("Tool %q before hook failed: %v", call.Name, err),
		)
		return newToolCallOutcome(call, result, true, timestamp), nil
	}
	if beforeResult.Block {
		reason := beforeResult.Reason
		if reason == "" {
			reason = "Tool execution was blocked"
		}
		result := newErrorToolResult(reason)
		result.Terminate = beforeResult.Terminate
		return newToolCallOutcome(call, result, true, timestamp), nil
	}

	updates := newToolUpdateGate(onUpdate)
	result, err := tool.Execute(ctx, preparedCall, updates.update)
	if updateError := updates.settle(); updateError != nil {
		return toolCallOutcome{}, updateError
	}
	isError := false
	if err != nil {
		result = newErrorToolResult(err.Error())
		isError = true
	}
	result, isError = runAfterToolCall(
		ctx, options, call, preparedCall, result, isError,
	)
	return newToolCallOutcome(call, result, isError, timestamp), nil
}

type toolCallOutcome struct {
	message   ToolResultMessage
	result    ToolResult
	isError   bool
	terminate bool
}

func newToolCallOutcome(
	call ToolCall,
	result ToolResult,
	isError bool,
	timestamp int64,
) toolCallOutcome {
	return toolCallOutcome{
		message:   newToolResultMessage(call, result, isError, timestamp),
		result:    result,
		isError:   isError,
		terminate: result.Terminate,
	}
}

// newErrorToolResult 创建可返回给模型的错误工具结果。
func newErrorToolResult(message string) ToolResult {
	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: message},
		},
		Details: map[string]any{},
	}
}

// newToolResultMessage 将工具执行结果转换为对话消息。
func newToolResultMessage(
	call ToolCall,
	result ToolResult,
	isError bool,
	timestamp int64,
) ToolResultMessage {
	content := result.Content
	if content == nil {
		content = []ToolResultContent{}
	}

	return ToolResultMessage{
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		Content:        content,
		Details:        result.Details,
		Usage:          result.Usage,
		AddedToolNames: result.AddedToolNames,
		IsError:        isError,
		Timestamp:      timestamp,
	}
}
