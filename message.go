package agent

// AgentMessage 表示 Agent transcript 中的一条消息。
type AgentMessage interface {
	isAgentMessage()
	toLLM() (Message, bool)
}

// Message 表示 Model 可以接收的一条核心消息。
//
// 未导出的 isMessage 方法限制消息类型只能由 agent 包定义。
type Message interface {
	AgentMessage
	isMessage()
}

// CustomMessage 表示应用层附加到 Agent transcript 的自定义消息。
type CustomMessage struct {
	Kind    string
	Payload any
}

func (CustomMessage) isAgentMessage() {}

func (CustomMessage) toLLM() (Message, bool) {
	return nil, false
}

// UserContent 表示用户消息中允许出现的内容。
//
// 用户只能提交文本和图片，不能直接提交 ThinkingContent 或 ToolCall。
type UserContent interface {
	Content
	isUserContent()
}

// UserMessage 表示用户发送给 Agent 的消息。
type UserMessage struct {
	Content   []UserContent
	Timestamp int64
}

// isMessage 将 UserMessage 标记为 Message 的一种实现。
func (UserMessage) isMessage() {}

func (UserMessage) isAgentMessage() {}

func (message UserMessage) toLLM() (Message, bool) {
	return message, true
}

// AssistantContent 表示 Assistant 消息中允许出现的内容。
type AssistantContent interface {
	Content
	isAssistantContent()
}

// AssistantMessage 表示模型生成的一条响应消息。
type AssistantMessage struct {
	Content       []AssistantContent
	API           string
	Provider      string
	Model         string
	ResponseModel string
	ResponseID    string
	Usage         Usage
	StopReason    StopReason
	ErrorMessage  string
	RawStopReason string
	Timestamp     int64
}

// isMessage 将 AssistantMessage 标记为 Message 的一种实现。
func (AssistantMessage) isMessage() {}

func (AssistantMessage) isAgentMessage() {}

func (message AssistantMessage) toLLM() (Message, bool) {
	return message, true
}

// ToolResultContent 表示工具结果消息中允许出现的内容。
type ToolResultContent interface {
	Content
	isToolResultContent()
}

// ToolResultMessage 表示一次工具调用的执行结果。
type ToolResultMessage struct {
	ToolCallID     string
	ToolName       string
	Content        []ToolResultContent
	Details        any
	Usage          *Usage
	AddedToolNames []string
	IsError        bool
	Timestamp      int64
}

// isMessage 将 ToolResultMessage 标记为 Message 的一种实现。
func (ToolResultMessage) isMessage() {}

func (ToolResultMessage) isAgentMessage() {}

func (message ToolResultMessage) toLLM() (Message, bool) {
	return message, true
}
