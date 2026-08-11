package agent

func cloneAssistantMessage(message AssistantMessage) AssistantMessage {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]AssistantContent, len(message.Content))
		for index, content := range message.Content {
			cloned.Content[index] = cloneAssistantContent(content)
		}
	}
	cloned.Usage.CacheWrite1HTokens = cloneInt64(message.Usage.CacheWrite1HTokens)
	cloned.Usage.ReasoningTokens = cloneInt64(message.Usage.ReasoningTokens)

	return cloned
}

func cloneAssistantContent(content AssistantContent) AssistantContent {
	switch value := content.(type) {
	case TextContent:
		return value
	case *TextContent:
		if value == nil {
			return value
		}

		cloned := *value

		return &cloned
	case ThinkingContent:
		return value
	case *ThinkingContent:
		if value == nil {
			return value
		}

		cloned := *value

		return &cloned
	case ToolCall:
		return cloneToolCall(value)
	case *ToolCall:
		if value == nil {
			return value
		}

		cloned := cloneToolCall(*value)

		return &cloned
	}

	return content
}

func cloneToolCall(call ToolCall) ToolCall {
	cloned := call
	cloned.Arguments = cloneArguments(call.Arguments)

	return cloned
}

func cloneArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}

	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = cloneArgumentValue(value)
	}

	return cloned
}

func cloneArgumentValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneArguments(value)
	case []any:
		if value == nil {
			return []any(nil)
		}

		cloned := make([]any, len(value))
		for index, item := range value {
			cloned[index] = cloneArgumentValue(item)
		}

		return cloned
	default:
		return value
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}
