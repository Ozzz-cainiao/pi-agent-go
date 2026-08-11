package agent

// AssistantDoneEvent 表示 Assistant 正常完成响应。
type AssistantDoneEvent struct {
	Reason  StopReason
	Message AssistantMessage
}

func (AssistantDoneEvent) isAssistantMessageEvent() {}

func (event AssistantDoneEvent) snapshot() AssistantMessageEvent {
	event.Message = cloneAssistantMessage(event.Message)

	return event
}

// AssistantErrorEvent 表示 Assistant 因失败或取消而结束响应。
type AssistantErrorEvent struct {
	Reason StopReason
	Error  AssistantMessage
}

func (AssistantErrorEvent) isAssistantMessageEvent() {}

func (event AssistantErrorEvent) snapshot() AssistantMessageEvent {
	event.Error = cloneAssistantMessage(event.Error)

	return event
}
