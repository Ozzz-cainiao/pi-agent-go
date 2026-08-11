package agent

// AssistantToolCallStartEvent 表示 Assistant 开始生成一次工具调用。
type AssistantToolCallStartEvent struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (AssistantToolCallStartEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallStartEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}

// AssistantToolCallDeltaEvent 表示 Assistant 生成了一段增量工具参数。
type AssistantToolCallDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (AssistantToolCallDeltaEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallDeltaEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}

// AssistantToolCallEndEvent 表示 Assistant 完成一次工具调用。
type AssistantToolCallEndEvent struct {
	ContentIndex int
	ToolCall     ToolCall
	Partial      AssistantMessage
}

func (AssistantToolCallEndEvent) isAssistantMessageEvent() {}

func (event AssistantToolCallEndEvent) snapshot() AssistantMessageEvent {
	event.ToolCall = cloneToolCall(event.ToolCall)
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}
