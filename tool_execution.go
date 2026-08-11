package agent

import (
	"context"
)

// executeToolCall 执行一次工具调用，并将结果转换为标准消息。
func executeToolCall(
	ctx context.Context,
	call ToolCall,
	timestamp int64,
	onUpdate toolUpdateSink,
	options toolCallExecutionOptions,
) (toolCallOutcome, error) {
	preparation := prepareToolCall(ctx, call, timestamp, options)
	if !preparation.execute {
		return preparation.immediate, nil
	}
	return executePreparedToolCall(ctx, preparation, onUpdate)
}

func executePreparedToolCall(
	ctx context.Context,
	preparation toolCallPreparation,
	onUpdate toolUpdateSink,
) (toolCallOutcome, error) {
	updates := newToolUpdateGate(onUpdate)
	result, err := preparation.tool.Execute(
		ctx,
		preparation.preparedCall,
		updates.update,
	)
	if updateError := updates.settle(); updateError != nil {
		return toolCallOutcome{}, updateError
	}
	isError := false
	if err != nil {
		result = newErrorToolResult(err.Error())
		isError = true
	}
	result, isError = runAfterToolCall(
		ctx,
		preparation.options,
		preparation.call,
		preparation.preparedCall,
		result,
		isError,
	)
	return newToolCallOutcome(
		preparation.call,
		result,
		isError,
		preparation.timestamp,
	), nil
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
