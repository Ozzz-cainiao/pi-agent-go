package agent

import "testing"

func TestAssistantMessageEventSink_clonesPartialSnapshots(t *testing.T) {
	reasoningTokens := int64(7)
	arguments := map[string]any{
		"query": "pi",
		"filters": map[string]any{
			"language": "go",
		},
	}
	partial := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:        "call-1",
				Name:      "search",
				Arguments: arguments,
			},
		},
		Usage: Usage{ReasoningTokens: &reasoningTokens},
	}

	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		delta, ok := event.(AssistantToolCallDeltaEvent)
		if !ok {
			t.Fatalf("event type = %T, want AssistantToolCallDeltaEvent", event)
		}
		call, ok := delta.Partial.Content[0].(ToolCall)
		if !ok {
			t.Fatalf("partial content type = %T, want ToolCall", delta.Partial.Content[0])
		}
		call.Arguments["query"] = "changed"
		filters, ok := call.Arguments["filters"].(map[string]any)
		if !ok {
			t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
		}
		filters["language"] = "typescript"
		delta.Partial.Content[0] = TextContent{Text: "changed"}
		*delta.Partial.Usage.ReasoningTokens = 99

		return nil
	})

	err := emit(AssistantToolCallDeltaEvent{
		ContentIndex: 0,
		Delta:        `{"query":"pi"}`,
		Partial:      partial,
	})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	call, ok := partial.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("partial content type = %T, want ToolCall", partial.Content[0])
	}
	if call.Arguments["query"] != "pi" {
		t.Fatalf("partial query = %v, want pi", call.Arguments["query"])
	}
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	if filters["language"] != "go" {
		t.Fatalf("partial language = %v, want go", filters["language"])
	}
	if reasoningTokens != 7 {
		t.Fatalf("partial reasoning tokens = %d, want 7", reasoningTokens)
	}
}

func TestAssistantMessageEventSink_clonesTerminalMessagesAndToolCalls(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call-1",
		Name: "search",
		Arguments: map[string]any{
			"query": "pi",
		},
	}
	message := AssistantMessage{
		Content: []AssistantContent{toolCall},
	}

	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		switch value := event.(type) {
		case AssistantToolCallEndEvent:
			value.ToolCall.Arguments["query"] = "changed tool call"
		case AssistantDoneEvent:
			call, ok := value.Message.Content[0].(ToolCall)
			if !ok {
				t.Fatalf("message content type = %T, want ToolCall", value.Message.Content[0])
			}
			call.Arguments["query"] = "changed message"
		}

		return nil
	})

	if err := emit(AssistantToolCallEndEvent{ToolCall: toolCall, Partial: message}); err != nil {
		t.Fatalf("emit tool call end returned error: %v", err)
	}
	if err := emit(AssistantDoneEvent{Reason: StopReasonStop, Message: message}); err != nil {
		t.Fatalf("emit done returned error: %v", err)
	}

	if toolCall.Arguments["query"] != "pi" {
		t.Fatalf("tool call query = %v, want pi", toolCall.Arguments["query"])
	}
	messageCall, ok := message.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("message content type = %T, want ToolCall", message.Content[0])
	}
	if messageCall.Arguments["query"] != "pi" {
		t.Fatalf("message query = %v, want pi", messageCall.Arguments["query"])
	}
}
