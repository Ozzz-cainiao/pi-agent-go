package agent

import "fmt"

type agentActiveRun struct {
	done   chan struct{}
	cancel func()
}

func applyAgentRuntimeCommand(
	owned *agentOwnedState,
	command agentStateCommand,
) agentStateReply {
	if command.operation == agentStateSubscribe {
		owned.nextSubscriberID++
		entry := agentSubscriberEntry{
			id: owned.nextSubscriberID, subscriber: command.subscriber,
		}
		owned.subscribers = append(owned.subscribers, entry)
		return agentStateReply{subscriberID: entry.id}
	}
	if command.operation == agentStateUnsubscribe {
		owned.subscribers = removeSubscriber(owned.subscribers, command.subscriberID)
		return agentStateReply{}
	}
	return applyAgentRunCommand(owned, command)
}

func applyAgentRunCommand(owned *agentOwnedState, command agentStateCommand) agentStateReply {
	if command.operation == agentStateBeginRun {
		return beginAgentRun(owned, command)
	}
	if command.operation == agentStateProcessEvent {
		return processAgentEvent(owned, command.event)
	}
	if command.operation == agentStateFinishRun {
		finishAgentRun(owned)
		return agentStateReply{}
	}
	if command.operation == agentStateCurrentIdle && owned.active != nil {
		return agentStateReply{idle: owned.active.done}
	}
	if command.operation == agentStateAbort && owned.active != nil {
		owned.active.cancel()
		return agentStateReply{}
	}
	if command.operation == agentStateReset {
		if owned.active != nil {
			return agentStateReply{err: ErrAgentBusy}
		}
		resetAgentRuntimeState(&owned.state)
		return agentStateReply{}
	}
	if command.operation == agentStateCurrentIdle || command.operation == agentStateAbort {
		return agentStateReply{}
	}
	return agentStateReply{err: fmt.Errorf("unsupported runtime state operation: %d", command.operation)}
}

func beginAgentRun(owned *agentOwnedState, command agentStateCommand) agentStateReply {
	if owned.active != nil {
		return agentStateReply{err: ErrAgentBusy}
	}
	snapshot, err := cloneAgentState(owned.state)
	if err != nil {
		return agentStateReply{err: fmt.Errorf("snapshot agent run state: %w", err)}
	}
	owned.active = &agentActiveRun{done: command.runDone, cancel: command.cancel}
	owned.state.IsStreaming = true
	owned.state.StreamingMessage = nil
	owned.state.PendingToolCalls = map[string]struct{}{}
	owned.state.ErrorMessage = ""
	return agentStateReply{state: snapshot}
}

func processAgentEvent(owned *agentOwnedState, event AgentEvent) agentStateReply {
	if err := reduceAgentEvent(&owned.state, event); err != nil {
		return agentStateReply{err: err}
	}
	return agentStateReply{
		subscribers: append([]agentSubscriberEntry(nil), owned.subscribers...),
	}
}

func finishAgentRun(owned *agentOwnedState) {
	owned.state.IsStreaming = false
	owned.state.StreamingMessage = nil
	owned.state.PendingToolCalls = map[string]struct{}{}
	if owned.active != nil {
		close(owned.active.done)
		owned.active = nil
	}
}

func resetAgentRuntimeState(state *AgentState) {
	state.Messages = []AgentMessage{}
	state.IsStreaming = false
	state.StreamingMessage = nil
	state.PendingToolCalls = map[string]struct{}{}
	state.ErrorMessage = ""
}

func removeSubscriber(
	subscribers []agentSubscriberEntry,
	subscriberID uint64,
) []agentSubscriberEntry {
	for index, entry := range subscribers {
		if entry.id == subscriberID {
			return append(subscribers[:index], subscribers[index+1:]...)
		}
	}
	return subscribers
}

func reduceAgentEvent(state *AgentState, event AgentEvent) error {
	snapshot, err := event.snapshot()
	if err != nil {
		return fmt.Errorf("snapshot agent state event: %w", err)
	}
	switch value := snapshot.(type) {
	case AgentStartEvent:
		state.ErrorMessage = ""
	case AgentMessageStartEvent:
		state.StreamingMessage = value.Message
	case AgentMessageUpdateEvent:
		state.StreamingMessage = value.Message
	case AgentMessageEndEvent:
		state.StreamingMessage = nil
		state.Messages = append(state.Messages, value.Message)
	case AgentToolExecutionStartEvent:
		state.PendingToolCalls[value.ToolCallID] = struct{}{}
	case AgentToolExecutionEndEvent:
		delete(state.PendingToolCalls, value.ToolCallID)
	case AgentTurnEndEvent:
		if value.Message.ErrorMessage != "" {
			state.ErrorMessage = value.Message.ErrorMessage
		}
	case AgentEndEvent:
		state.StreamingMessage = nil
	case AgentTurnStartEvent, AgentToolExecutionUpdateEvent:
	}
	return nil
}
