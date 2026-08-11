package agent

import (
	"context"
	"errors"
)

type agentRunEventDispatcher struct {
	agent    *Agent
	ctx      context.Context
	failures *[]error
	terminal *AgentEndEvent
}

func (dispatcher *agentRunEventDispatcher) handle(event AgentEvent) error {
	if terminal, ok := event.(AgentEndEvent); ok {
		dispatcher.terminal = &terminal
		return nil
	}
	return dispatcher.publish(event)
}

func (dispatcher *agentRunEventDispatcher) complete(runError error, model ModelID) error {
	if runError != nil {
		failure := dispatcher.agent.newFailureMessage(runError, model)
		return dispatcher.publishAll([]AgentEvent{
			AgentMessageStartEvent{Message: failure},
			AgentMessageEndEvent{Message: failure},
			AgentTurnEndEvent{Message: failure},
			AgentEndEvent{Messages: []AgentMessage{failure}},
		})
	}
	if dispatcher.terminal == nil {
		return dispatcher.publish(AgentEndEvent{})
	}
	return dispatcher.publish(*dispatcher.terminal)
}

func (dispatcher *agentRunEventDispatcher) publishAll(events []AgentEvent) error {
	failures := make([]error, 0)
	for _, event := range events {
		if err := dispatcher.publish(event); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (dispatcher *agentRunEventDispatcher) publish(event AgentEvent) error {
	failures, err := dispatcher.agent.dispatchEvent(dispatcher.ctx, event)
	*dispatcher.failures = append(*dispatcher.failures, failures...)
	return err
}

func (agent *Agent) newFailureMessage(runError error, model ModelID) AssistantMessage {
	reason := StopReasonError
	if errors.Is(runError, context.Canceled) || errors.Is(runError, context.DeadlineExceeded) {
		reason = StopReasonAborted
	}
	return AssistantMessage{
		Content:      []AssistantContent{TextContent{Text: ""}},
		Model:        string(model),
		StopReason:   reason,
		ErrorMessage: runError.Error(),
		Timestamp:    resolvedClock(agent.loopConfig.Clock)().UnixMilli(),
	}
}
