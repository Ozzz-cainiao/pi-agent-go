package agent

import "reflect"

func cloneAssistantMessage(message AssistantMessage) (AssistantMessage, error) {
	return newSnapshotCloner().cloneAssistantMessage(message)
}

func (cloner *snapshotCloner) cloneAssistantMessage(
	message AssistantMessage,
) (AssistantMessage, error) {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]AssistantContent, len(message.Content))
		for index, content := range message.Content {
			clonedContent, err := cloner.cloneAssistantContent(content)
			if err != nil {
				return AssistantMessage{}, err
			}
			cloned.Content[index] = clonedContent
		}
	}
	cloned.Usage.CacheWrite1HTokens = cloneInt64(message.Usage.CacheWrite1HTokens)
	cloned.Usage.ReasoningTokens = cloneInt64(message.Usage.ReasoningTokens)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneAssistantContent(
	content AssistantContent,
) (AssistantContent, error) {
	switch value := content.(type) {
	case TextContent:
		return value, nil
	case *TextContent:
		if value == nil {
			return value, nil
		}

		cloned := *value

		return &cloned, nil
	case ThinkingContent:
		return value, nil
	case *ThinkingContent:
		if value == nil {
			return value, nil
		}

		cloned := *value

		return &cloned, nil
	case ToolCall:
		return cloner.cloneToolCall(value)
	case *ToolCall:
		if value == nil {
			return value, nil
		}

		cloned, err := cloner.cloneToolCall(*value)
		if err != nil {
			return nil, err
		}

		return &cloned, nil
	}

	return content, nil
}

func (cloner *snapshotCloner) cloneToolCall(call ToolCall) (ToolCall, error) {
	clonedArguments, err := cloner.cloneValue(
		reflect.ValueOf(call.Arguments),
		"ToolCall.Arguments",
	)
	if err != nil {
		return ToolCall{}, err
	}
	arguments, ok := clonedArguments.Interface().(map[string]any)
	if !ok {
		return ToolCall{}, &SnapshotCloneError{
			Path: "ToolCall.Arguments",
			Type: clonedArguments.Type().String(),
		}
	}

	cloned := call
	cloned.Arguments = arguments

	return cloned, nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}
