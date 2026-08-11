package agent

// AssistantStartEvent 表示 Assistant 开始生成响应。
type AssistantStartEvent struct {
	Partial AssistantMessage
}

func (AssistantStartEvent) isAssistantMessageEvent() {}

func (event AssistantStartEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}

// AssistantTextStartEvent 表示 Assistant 开始生成一段文本。
type AssistantTextStartEvent struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (AssistantTextStartEvent) isAssistantMessageEvent() {}

func (event AssistantTextStartEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}

// AssistantTextDeltaEvent 表示 Assistant 生成了一段增量文本。
type AssistantTextDeltaEvent struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (AssistantTextDeltaEvent) isAssistantMessageEvent() {}

func (event AssistantTextDeltaEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}

// AssistantTextEndEvent 表示 Assistant 完成一段文本。
type AssistantTextEndEvent struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (AssistantTextEndEvent) isAssistantMessageEvent() {}

func (event AssistantTextEndEvent) snapshot() AssistantMessageEvent {
	event.Partial = cloneAssistantMessage(event.Partial)

	return event
}
