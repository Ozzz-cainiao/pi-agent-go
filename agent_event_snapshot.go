package agent

import "reflect"

func (event AgentStartEvent) snapshot() (AgentEvent, error)     { return event, nil }
func (event AgentTurnStartEvent) snapshot() (AgentEvent, error) { return event, nil }

func (event AgentEndEvent) snapshot() (AgentEvent, error) {
	messages, err := cloneAgentMessages(event.Messages)
	if err != nil {
		return nil, err
	}
	event.Messages = messages
	return event, nil
}

func (event AgentTurnEndEvent) snapshot() (AgentEvent, error) {
	cloner := newSnapshotCloner()
	message, err := cloner.cloneAssistantMessage(event.Message)
	if err != nil {
		return nil, err
	}
	results := make([]ToolResultMessage, len(event.ToolResults))
	for index, result := range event.ToolResults {
		results[index], err = cloner.cloneToolResultMessage(result)
		if err != nil {
			return nil, err
		}
	}
	event.Message = message
	event.ToolResults = results
	return event, nil
}

func (event AgentMessageStartEvent) snapshot() (AgentEvent, error) {
	message, err := newSnapshotCloner().cloneAgentMessage(event.Message)
	if err != nil {
		return nil, err
	}
	event.Message = message
	return event, nil
}

func (event AgentMessageUpdateEvent) snapshot() (AgentEvent, error) {
	message, err := cloneAssistantMessage(event.Message)
	if err != nil {
		return nil, err
	}
	assistantEvent, err := event.AssistantEvent.snapshot()
	if err != nil {
		return nil, err
	}
	event.Message = message
	event.AssistantEvent = assistantEvent
	return event, nil
}

func (event AgentMessageEndEvent) snapshot() (AgentEvent, error) {
	message, err := newSnapshotCloner().cloneAgentMessage(event.Message)
	if err != nil {
		return nil, err
	}
	event.Message = message
	return event, nil
}

func (event AgentToolExecutionStartEvent) snapshot() (AgentEvent, error) {
	arguments, err := cloneArguments(event.Arguments)
	if err != nil {
		return nil, err
	}
	event.Arguments = arguments
	return event, nil
}

func (event AgentToolExecutionUpdateEvent) snapshot() (AgentEvent, error) {
	cloner := newSnapshotCloner()
	arguments, err := cloneArguments(event.Arguments)
	if err != nil {
		return nil, err
	}
	result, err := cloner.cloneToolResult(event.PartialResult)
	if err != nil {
		return nil, err
	}
	event.Arguments = arguments
	event.PartialResult = result
	return event, nil
}

func (event AgentToolExecutionEndEvent) snapshot() (AgentEvent, error) {
	result, err := newSnapshotCloner().cloneToolResult(event.Result)
	if err != nil {
		return nil, err
	}
	event.Result = result
	return event, nil
}

func cloneAgentMessages(messages []AgentMessage) ([]AgentMessage, error) {
	cloner := newSnapshotCloner()
	cloned := make([]AgentMessage, len(messages))
	for index, message := range messages {
		var err error
		cloned[index], err = cloner.cloneAgentMessage(message)
		if err != nil {
			return nil, err
		}
	}
	return cloned, nil
}

func cloneArguments(arguments map[string]any) (map[string]any, error) {
	if arguments == nil {
		return map[string]any{}, nil
	}
	value, err := newSnapshotCloner().cloneValue(
		reflect.ValueOf(arguments),
		"ToolCall.Arguments",
	)
	if err != nil {
		return nil, err
	}
	cloned, ok := value.Interface().(map[string]any)
	if !ok {
		return nil, newSnapshotCloneError("ToolCall.Arguments", value.Type())
	}
	return cloned, nil
}
