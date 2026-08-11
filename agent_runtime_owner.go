package agent

import "fmt"

func applyAgentRuntimeCommand(
	owned *agentOwnedState,
	command agentStateCommand,
) agentStateReply {
	switch command.operation {
	case agentStateSubscribe:
		owned.nextSubscriberID++
		entry := agentSubscriberEntry{
			id: owned.nextSubscriberID, subscriber: command.subscriber,
		}
		owned.subscribers = append(owned.subscribers, entry)
		return agentStateReply{subscriberID: entry.id}
	case agentStateUnsubscribe:
		owned.subscribers = removeSubscriber(owned.subscribers, command.subscriberID)
	case agentStateBeginRun:
		if owned.activeDone != nil {
			return agentStateReply{err: ErrAgentBusy}
		}
		snapshot, err := cloneAgentState(owned.state)
		if err != nil {
			return agentStateReply{err: fmt.Errorf("snapshot agent run state: %w", err)}
		}
		owned.activeDone = command.runDone
		owned.state.IsStreaming = true
		owned.state.StreamingMessage = nil
		owned.state.PendingToolCalls = map[string]struct{}{}
		owned.state.ErrorMessage = ""
		return agentStateReply{state: snapshot}
	case agentStateProcessEvent:
		if err := reduceAgentEvent(&owned.state, command.event); err != nil {
			return agentStateReply{err: err}
		}
		return agentStateReply{
			subscribers: append([]agentSubscriberEntry(nil), owned.subscribers...),
		}
	case agentStateFinishRun:
		owned.state.IsStreaming = false
		owned.state.StreamingMessage = nil
		owned.state.PendingToolCalls = map[string]struct{}{}
		if owned.activeDone != nil {
			close(owned.activeDone)
			owned.activeDone = nil
		}
	case agentStateCurrentIdle:
		return agentStateReply{idle: owned.activeDone}
	case agentStateSnapshot,
		agentStateSetSystemPrompt,
		agentStateSetModel,
		agentStateSetThinkingLevel,
		agentStateSetTools,
		agentStateReplaceMessages,
		agentStateAppendMessage,
		agentStateClearMessages,
		agentStateClose:
		return agentStateReply{err: fmt.Errorf("unsupported runtime state operation: %d", command.operation)}
	}
	return agentStateReply{}
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
