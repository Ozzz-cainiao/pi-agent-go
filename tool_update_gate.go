package agent

import "sync"

type toolUpdateSink func(ToolResult) error

type toolUpdateGate struct {
	mutex     sync.Mutex
	accepting bool
	sink      toolUpdateSink
	err       error
}

func newToolUpdateGate(sink toolUpdateSink) *toolUpdateGate {
	return &toolUpdateGate{accepting: true, sink: sink}
}

func (gate *toolUpdateGate) update(partial ToolResult) {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if !gate.accepting || gate.err != nil || gate.sink == nil {
		return
	}
	gate.err = gate.sink(partial)
}

func (gate *toolUpdateGate) settle() error {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	gate.accepting = false
	return gate.err
}
