package agent

// Content 表示模型生成的一段结构化内容。
//
// 未导出的 isContent 方法限制 Content 只能由 agent 包内定义的类型实现，
// 从而模拟 TypeScript 的联合类型。
type Content interface {
	isContent()
}

// TextContent 表示模型生成的文本内容。
type TextContent struct {
	Text          string
	TextSignature string
}

// ThinkingContent 表示模型生成的推理内容。
type ThinkingContent struct {
	Thinking          string
	ThinkingSignature string
	Redacted          bool
}

// ImageContent 表示以 base64 编码的图片内容。
type ImageContent struct {
	Data     string
	MIMEType string
}

// ToolCall 表示模型发起的一次工具调用。
type ToolCall struct {
	ID               string
	Name             string
	Arguments        map[string]any
	ThoughtSignature string
	Namespace        string
}

// isContent 将 TextContent 标记为 Content 的一种实现。
func (TextContent) isContent() {}

// isContent 将 ThinkingContent 标记为 Content 的一种实现。
func (ThinkingContent) isContent() {}

// isContent 将 ImageContent 标记为 Content 的一种实现。
func (ImageContent) isContent() {}

// isContent 将 ToolCall 标记为 Content 的一种实现。
func (ToolCall) isContent() {}

// isUserContent 将 TextContent 标记为可用于用户消息的内容。
func (TextContent) isUserContent() {}

// isUserContent 将 ImageContent 标记为可用于用户消息的内容。
func (ImageContent) isUserContent() {}

// isAssistantContent 将 TextContent 标记为可用于 Assistant 消息的内容。
func (TextContent) isAssistantContent() {}

// isAssistantContent 将 ThinkingContent 标记为可用于 Assistant 消息的内容。
func (ThinkingContent) isAssistantContent() {}

// isAssistantContent 将 ToolCall 标记为可用于 Assistant 消息的内容。
func (ToolCall) isAssistantContent() {}

// isToolResultContent 将 TextContent 标记为可用于工具结果消息的内容。
func (TextContent) isToolResultContent() {}

// isToolResultContent 将 ImageContent 标记为可用于工具结果消息的内容。
func (ImageContent) isToolResultContent() {}
