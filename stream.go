package agent

import "context"

// AssistantMessageEvent 表示从 start 到 done/error 的一个 Assistant 流式事件。
type AssistantMessageEvent interface {
	isAssistantMessageEvent()
	snapshot() AssistantMessageEvent
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

func assistantMessageEventSinkOrDiscard(
	emit AssistantMessageEventSink,
) AssistantMessageEventSink {
	if emit != nil {
		return func(event AssistantMessageEvent) error {
			return emit(event.snapshot())
		}
	}

	return func(AssistantMessageEvent) error {
		return nil
	}
}
