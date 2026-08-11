package agent

func applyAgentQueueCommand(
	owned *agentOwnedState,
	command agentStateCommand,
) agentStateReply {
	if command.operation == agentStateEnqueueSteering {
		owned.steeringQueue.messages = append(owned.steeringQueue.messages, command.queueMessage)
		return agentStateReply{}
	}
	if command.operation == agentStateEnqueueFollowUp {
		owned.followUpQueue.messages = append(owned.followUpQueue.messages, command.queueMessage)
		return agentStateReply{}
	}
	if command.operation == agentStateDrainSteering {
		return agentStateReply{queueMessages: owned.steeringQueue.drain()}
	}
	if command.operation == agentStateDrainFollowUp {
		return agentStateReply{queueMessages: owned.followUpQueue.drain()}
	}
	if command.operation == agentStateSetSteeringMode {
		owned.steeringQueue.mode = command.queueMode
		return agentStateReply{}
	}
	if command.operation == agentStateSetFollowUpMode {
		owned.followUpQueue.mode = command.queueMode
		return agentStateReply{}
	}
	if command.operation == agentStateHasQueuedMessages {
		return agentStateReply{
			hasQueued: len(owned.steeringQueue.messages) > 0 || len(owned.followUpQueue.messages) > 0,
		}
	}
	return agentStateReply{}
}

func (queue *pendingMessageQueue) drain() []AgentMessage {
	if len(queue.messages) == 0 {
		return nil
	}
	count := 1
	if queue.mode == QueueModeAll {
		count = len(queue.messages)
	}
	drained := append([]AgentMessage(nil), queue.messages[:count]...)
	queue.messages = append([]AgentMessage(nil), queue.messages[count:]...)
	return drained
}
