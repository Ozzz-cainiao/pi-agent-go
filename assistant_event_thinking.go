package agent

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
