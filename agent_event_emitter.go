package agent

type agentEventEmitter struct {
	sink AgentEventSink
}

func newAgentEventEmitter(sink AgentEventSink) agentEventEmitter {
	return agentEventEmitter{sink: sink}
}

func (emitter agentEventEmitter) emit(event AgentEvent) error {
	if emitter.sink == nil {
		return nil
	}
	snapshot, err := event.snapshot()
	if err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	if err := emitter.sink(snapshot); err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	return nil
}
