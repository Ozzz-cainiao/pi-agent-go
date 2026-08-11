package agent

import "sync"

type agentEventEmitter struct {
	sink  AgentEventSink
	mutex *sync.Mutex
}

func newAgentEventEmitter(sink AgentEventSink) agentEventEmitter {
	return agentEventEmitter{sink: sink, mutex: &sync.Mutex{}}
}

func (emitter agentEventEmitter) emit(event AgentEvent) error {
	if emitter.sink == nil {
		return nil
	}
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	snapshot, err := event.snapshot()
	if err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	if err := emitter.sink(snapshot); err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	return nil
}
