package agent

import (
	"errors"
	"fmt"
)

var ErrAgentEventSink = errors.New("agent: event sink failed")

// AgentEventSinkError 描述事件快照或消费者处理失败。
type AgentEventSinkError struct {
	Kind  AgentEventKind
	Cause error
}

func (sinkError *AgentEventSinkError) Error() string {
	return fmt.Sprintf("agent: emit %s event: %v", sinkError.Kind, sinkError.Cause)
}

func (sinkError *AgentEventSinkError) Unwrap() []error {
	return []error{ErrAgentEventSink, sinkError.Cause}
}
