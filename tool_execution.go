package agent

import "context"

// executeToolCall 执行一次工具调用，并将结果转换为标准消息。
func executeToolCall(
	ctx context.Context,
	tools []Tool,
	call ToolCall,
	timestamp int64,
	onUpdate ToolUpdateFunc,
) ToolResultMessage {
	tool, ok := findToolByName(tools, call.Name)
	if !ok {
		result := newErrorToolResult(
			"Tool " + call.Name + " not found",
		)

		return newToolResultMessage(call, result, true, timestamp)
	}

	if onUpdate == nil {
		onUpdate = func(ToolResult) {}
	}
	result, err := tool.Execute(ctx, call, onUpdate)
	if err != nil {
		result = newErrorToolResult(err.Error())

		return newToolResultMessage(call, result, true, timestamp)
	}

	return newToolResultMessage(call, result, false, timestamp)
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
