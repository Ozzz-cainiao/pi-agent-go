package agent

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
