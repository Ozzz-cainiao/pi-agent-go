package agent

import "reflect"

func (cloner *snapshotCloner) cloneAgentMessage(message AgentMessage) (AgentMessage, error) {
	if message == nil {
		return nil, &SnapshotCloneError{Path: "AgentMessage", Type: "<nil>"}
	}
	messageValue := reflect.ValueOf(message)
	if messageValue.Kind() == reflect.Pointer {
		cloned, err := cloner.cloneValue(messageValue, "AgentMessage")
		if err != nil {
			return nil, err
		}
		clonedMessage, ok := cloned.Interface().(AgentMessage)
		if !ok {
			return nil, newSnapshotCloneError("AgentMessage", cloned.Type())
		}
		return clonedMessage, nil
	}

	switch value := message.(type) {
	case UserMessage:
		return cloner.cloneUserMessage(value)
	case AssistantMessage:
		return cloner.cloneAssistantMessage(value)
	case ToolResultMessage:
		return cloner.cloneToolResultMessage(value)
	case CustomMessage:
		if value.Payload != nil {
			payload, err := cloner.cloneAny(value.Payload, "CustomMessage.Payload")
			if err != nil {
				return nil, err
			}
			value.Payload = payload
		}
		return value, nil
	}

	return message, nil
}

func (cloner *snapshotCloner) cloneUserMessage(message UserMessage) (UserMessage, error) {
	cloned := message
	cloned.Content = make([]UserContent, len(message.Content))
	for index, content := range message.Content {
		switch value := content.(type) {
		case *TextContent:
			if value != nil {
				copy := *value
				cloned.Content[index] = &copy
			}
		case *ImageContent:
			if value != nil {
				copy := *value
				cloned.Content[index] = &copy
			}
		default:
			cloned.Content[index] = content
		}
	}
	return cloned, nil
}

func (cloner *snapshotCloner) cloneToolResultMessage(message ToolResultMessage) (ToolResultMessage, error) {
	cloned := message
	cloned.Content = cloneToolResultContent(message.Content)
	if message.Details != nil {
		details, err := cloner.cloneAny(message.Details, "ToolResultMessage.Details")
		if err != nil {
			return ToolResultMessage{}, err
		}
		cloned.Details = details
	}
	cloned.Usage = cloneUsage(message.Usage)
	cloned.AddedToolNames = append([]string(nil), message.AddedToolNames...)
	return cloned, nil
}

func (cloner *snapshotCloner) cloneToolResult(result ToolResult) (ToolResult, error) {
	cloned := result
	cloned.Content = cloneToolResultContent(result.Content)
	if result.Details != nil {
		details, err := cloner.cloneAny(result.Details, "ToolResult.Details")
		if err != nil {
			return ToolResult{}, err
		}
		cloned.Details = details
	}
	cloned.Usage = cloneUsage(result.Usage)
	cloned.AddedToolNames = append([]string(nil), result.AddedToolNames...)
	return cloned, nil
}

func cloneToolResultContent(content []ToolResultContent) []ToolResultContent {
	cloned := make([]ToolResultContent, len(content))
	for index, item := range content {
		switch value := item.(type) {
		case *TextContent:
			if value != nil {
				copy := *value
				cloned[index] = &copy
			}
		case *ImageContent:
			if value != nil {
				copy := *value
				cloned[index] = &copy
			}
		default:
			cloned[index] = item
		}
	}
	return cloned
}

func (cloner *snapshotCloner) cloneAny(value any, path string) (any, error) {
	cloned, err := cloner.cloneValue(reflect.ValueOf(value), path)
	if err != nil {
		return nil, err
	}
	return cloned.Interface(), nil
}

func cloneUsage(usage *Usage) *Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	cloned.CacheWrite1HTokens = cloneInt64(usage.CacheWrite1HTokens)
	cloned.ReasoningTokens = cloneInt64(usage.ReasoningTokens)
	return &cloned
}
