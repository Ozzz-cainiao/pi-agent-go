package agent

import "context"

// AssistantMessageEvent 表示从 start 到 done/error 的一个 Assistant 流式事件。
type AssistantMessageEvent interface {
	isAssistantMessageEvent()
	snapshot() (AssistantMessageEvent, error)
}

// AssistantMessageEventSink 接收模型流中产生的事件。
type AssistantMessageEventSink func(AssistantMessageEvent) error

// StreamFunc 是 Agent Core 与 Model Provider 之间最重要的边界。
//
// Provider 负责把厂商协议转换成这些稳定类型：生成过程中通过 emit 发送流式事件，
// 结束时返回一条完整 AssistantMessage。Core 因此不需要了解 HTTP、SSE 或厂商 JSON。
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
			// 每次交付独立快照，消费者修改 Partial 或 ToolCall.Arguments 时不会污染
			// Provider 正在累积的下一帧消息。
			snapshot, err := event.snapshot()
			if err != nil {
				return err
			}

			return emit(snapshot)
		}
	}

	return func(AssistantMessageEvent) error {
		return nil
	}
}

// AssistantStartEvent 表示 Assistant 开始生成响应。
type AssistantStartEvent struct {
	Partial AssistantMessage
}

func (AssistantStartEvent) isAssistantMessageEvent() {}

func (event AssistantStartEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantTextStartEvent 表示 Assistant 开始生成一段文本。
type AssistantTextStartEvent struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (AssistantTextStartEvent) isAssistantMessageEvent() {}

func (event AssistantTextStartEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantTextDeltaEvent 表示 Assistant 生成了一段增量文本。
type AssistantTextDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (AssistantTextDeltaEvent) isAssistantMessageEvent() {}

func (event AssistantTextDeltaEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantTextEndEvent 表示 Assistant 完成一段文本。
type AssistantTextEndEvent struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (AssistantTextEndEvent) isAssistantMessageEvent() {}

func (event AssistantTextEndEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantThinkingStartEvent 表示 Assistant 开始生成一段推理内容。
type AssistantThinkingStartEvent struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (AssistantThinkingStartEvent) isAssistantMessageEvent() {}

func (event AssistantThinkingStartEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantThinkingDeltaEvent 表示 Assistant 生成了一段增量推理内容。
type AssistantThinkingDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (AssistantThinkingDeltaEvent) isAssistantMessageEvent() {}

func (event AssistantThinkingDeltaEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantThinkingEndEvent 表示 Assistant 完成一段推理内容。
type AssistantThinkingEndEvent struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (AssistantThinkingEndEvent) isAssistantMessageEvent() {}

func (event AssistantThinkingEndEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantToolCallStartEvent 表示 Assistant 开始生成一次工具调用。
type AssistantToolCallStartEvent struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (AssistantToolCallStartEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallStartEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantToolCallDeltaEvent 表示 Assistant 生成了一段增量工具参数。
type AssistantToolCallDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (AssistantToolCallDeltaEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallDeltaEvent) snapshot() (AssistantMessageEvent, error) {
	partial, err := cloneAssistantMessage(event.Partial)
	event.Partial = partial

	return event, err
}

// AssistantToolCallEndEvent 表示 Assistant 完成一次工具调用。
type AssistantToolCallEndEvent struct {
	ContentIndex int
	ToolCall     ToolCall
	Partial      AssistantMessage
}

func (AssistantToolCallEndEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallEndEvent) snapshot() (AssistantMessageEvent, error) {
	cloner := newSnapshotCloner()
	toolCall, err := cloner.cloneToolCall(event.ToolCall)
	if err != nil {
		return nil, err
	}
	partial, err := cloner.cloneAssistantMessage(event.Partial)
	event.ToolCall = toolCall
	event.Partial = partial

	return event, err
}

// AssistantDoneEvent 表示 Assistant 正常完成响应。
type AssistantDoneEvent struct {
	Reason  StopReason
	Message AssistantMessage
}

func (AssistantDoneEvent) isAssistantMessageEvent() {}

func (event AssistantDoneEvent) snapshot() (AssistantMessageEvent, error) {
	message, err := cloneAssistantMessage(event.Message)
	event.Message = message

	return event, err
}

// AssistantErrorEvent 表示 Assistant 因失败或取消而结束响应。
type AssistantErrorEvent struct {
	Reason StopReason
	Error  AssistantMessage
}

func (AssistantErrorEvent) isAssistantMessageEvent() {}

func (event AssistantErrorEvent) snapshot() (AssistantMessageEvent, error) {
	message, err := cloneAssistantMessage(event.Error)
	event.Error = message

	return event, err
}

type assistantLifecycle struct {
	events  agentEventEmitter
	started bool
	ended   bool
}

func (lifecycle *assistantLifecycle) sink() AssistantMessageEventSink {
	return func(event AssistantMessageEvent) error {
		// Provider 事件比 Agent 事件更细：text/thinking/tool_call delta 都统一映射为
		// message_update，而 start/done/error 用于闭合一条 transcript 消息。
		switch value := event.(type) {
		case AssistantStartEvent:
			lifecycle.started = true
			return lifecycle.events.emit(AgentMessageStartEvent{Message: value.Partial})
		case AssistantDoneEvent:
			lifecycle.ended = true
			return lifecycle.events.emit(AgentMessageEndEvent{Message: value.Message})
		case AssistantErrorEvent:
			lifecycle.ended = true
			return lifecycle.events.emit(AgentMessageEndEvent{Message: value.Error})
		default:
			partial, ok := assistantEventPartial(event)
			if !ok {
				return nil
			}
			return lifecycle.events.emit(AgentMessageUpdateEvent{
				Message:        partial,
				AssistantEvent: event,
			})
		}
	}
}

func (lifecycle *assistantLifecycle) finish(message AssistantMessage) error {
	if !lifecycle.started {
		if err := lifecycle.events.emit(AgentMessageStartEvent{Message: message}); err != nil {
			return err
		}
	}
	if lifecycle.ended {
		return nil
	}
	return lifecycle.events.emit(AgentMessageEndEvent{Message: message})
}

func assistantEventPartial(event AssistantMessageEvent) (AssistantMessage, bool) {
	switch value := event.(type) {
	case AssistantTextStartEvent:
		return value.Partial, true
	case AssistantTextDeltaEvent:
		return value.Partial, true
	case AssistantTextEndEvent:
		return value.Partial, true
	case AssistantThinkingStartEvent:
		return value.Partial, true
	case AssistantThinkingDeltaEvent:
		return value.Partial, true
	case AssistantThinkingEndEvent:
		return value.Partial, true
	case AssistantToolCallStartEvent:
		return value.Partial, true
	case AssistantToolCallDeltaEvent:
		return value.Partial, true
	case AssistantToolCallEndEvent:
		return value.Partial, true
	default:
		return AssistantMessage{}, false
	}
}
