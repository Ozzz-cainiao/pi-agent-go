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

// AssistantStartEvent 表示模型开始生成响应。
type AssistantStartEvent struct {
	Partial AssistantMessage
}

// isAssistantMessageEvent 将 AssistantStartEvent 标记为流式事件。
func (AssistantStartEvent) isAssistantMessageEvent() {}

// AssistantTextDeltaEvent 表示模型生成了一段增量文本。
type AssistantTextDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

// isAssistantMessageEvent 将 AssistantTextDeltaEvent 标记为流式事件。
func (AssistantTextDeltaEvent) isAssistantMessageEvent() {}
