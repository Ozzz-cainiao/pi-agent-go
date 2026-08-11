package agent

import "context"

// AssistantMessageEvent 表示模型流中产生的一个事件。
//
// 具体事件类型将在后续流式响应阶段逐步加入。
type AssistantMessageEvent interface {
	isAssistantMessageEvent()
}

// AssistantMessageEventSink 接收模型流中产生的事件。
type AssistantMessageEventSink func(AssistantMessageEvent) error

// StreamFunc 调用 Model，并返回最终的 AssistantMessage。
//
// Model 可以在返回最终消息前，通过 emit 发送流式事件。
type StreamFunc func(
	ctx context.Context,
	agentContext AgentContext,
	emit AssistantMessageEventSink,
) (AssistantMessage, error)
