package agent

type assistantLifecycle struct {
	events  agentEventEmitter
	started bool
	ended   bool
}

func (lifecycle *assistantLifecycle) sink() AssistantMessageEventSink {
	return func(event AssistantMessageEvent) error {
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
