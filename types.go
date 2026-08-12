package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

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

// UsageCost 表示一次模型响应产生的费用。
type UsageCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
	Total      float64
}

// Usage 表示一次模型响应的 token 使用量和费用。
type Usage struct {
	InputTokens        int64
	OutputTokens       int64
	CacheReadTokens    int64
	CacheWriteTokens   int64
	CacheWrite1HTokens *int64
	ReasoningTokens    *int64
	TotalTokens        int64
	Cost               UsageCost
}

// StopReason 表示模型响应停止的原因。
type StopReason string

const (
	// StopReasonPending 表示响应仍在流式生成。
	StopReasonPending StopReason = "pending"

	// StopReasonStop 表示模型正常完成响应。
	StopReasonStop StopReason = "stop"

	// StopReasonLength 表示模型达到长度限制。
	StopReasonLength StopReason = "length"

	// StopReasonToolUse 表示模型请求执行工具。
	StopReasonToolUse StopReason = "toolUse"

	// StopReasonError 表示模型执行失败。
	StopReasonError StopReason = "error"

	// StopReasonAborted 表示执行被取消。
	StopReasonAborted StopReason = "aborted"

	// StopReasonDeferred 表示响应需要稍后获取。
	StopReasonDeferred StopReason = "deferred"
)

// ErrInvalidStopReason 表示不受支持的 StopReason。
var ErrInvalidStopReason = errors.New("agent: invalid stop reason")

// ParseStopReason 将外部字符串解析为合法的 StopReason。
func ParseStopReason(raw string) (StopReason, error) {
	reason := StopReason(raw)

	switch reason {
	case StopReasonPending,
		StopReasonStop,
		StopReasonLength,
		StopReasonToolUse,
		StopReasonError,
		StopReasonAborted,
		StopReasonDeferred:
		return reason, nil
	}

	return "", fmt.Errorf("parse stop reason %q: %w", raw, ErrInvalidStopReason)
}

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

// AgentContext 表示传递给 Agent Loop 的上下文快照。
type AgentContext struct {
	SystemPrompt string
	Messages     []AgentMessage
	Tools        []Tool
}

// WithMessages 返回追加消息后的独立上下文快照。
//
// 原有 AgentContext 及其 Messages slice 不会被修改。
func (current AgentContext) WithMessages(messages ...AgentMessage) AgentContext {
	next := current
	next.Messages = slices.Clone(current.Messages)
	next.Messages = append(next.Messages, messages...)
	next.Tools = slices.Clone(current.Tools)

	return next
}

func (current AgentContext) toLLM(
	ctx context.Context,
	transform TransformContextFunc,
	convert ConvertToLLMFunc,
) (AgentContext, error) {
	messages, err := cloneAgentMessages(current.Messages)
	if err != nil {
		return AgentContext{}, err
	}
	if transform != nil {
		messages, err = transform(ctx, messages)
		if err != nil {
			return AgentContext{}, err
		}
	}
	converted, err := convert(ctx, messages)
	if err != nil {
		return AgentContext{}, err
	}

	next := current
	next.Messages = make([]AgentMessage, len(converted))
	for index, message := range converted {
		next.Messages[index] = message
	}
	next.Tools = slices.Clone(current.Tools)

	return next, nil
}
