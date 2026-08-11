package agent

import "testing"

func TestAssistantMessageEventSink_clonesCommonMutableArgumentValues(t *testing.T) {
	tags := []string{"agent", "go"}
	labels := map[string]string{"language": "go"}
	payload := []byte{1, 2, 3}
	nested := []any{map[string]string{"scope": "core"}}
	partial := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				Arguments: map[string]any{
					"tags":    tags,
					"labels":  labels,
					"payload": payload,
					"nested":  nested,
				},
			},
		},
	}
	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		delta, ok := event.(AssistantToolCallDeltaEvent)
		if !ok {
			t.Fatalf("event type = %T, want AssistantToolCallDeltaEvent", event)
		}
		call := contentAt[ToolCall](t, delta.Partial, 0)
		argumentAs[[]string](t, call.Arguments, "tags")[0] = "changed"
		argumentAs[map[string]string](t, call.Arguments, "labels")["language"] = "typescript"
		argumentAs[[]byte](t, call.Arguments, "payload")[0] = 9
		nestedValues := argumentAs[[]any](t, call.Arguments, "nested")
		valueAs[map[string]string](t, nestedValues[0])["scope"] = "changed"

		return nil
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: partial})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	if tags[0] != "agent" {
		t.Fatalf("tags[0] = %q, want agent", tags[0])
	}
	if labels["language"] != "go" {
		t.Fatalf("labels[language] = %q, want go", labels["language"])
	}
	if payload[0] != 1 {
		t.Fatalf("payload[0] = %d, want 1", payload[0])
	}
	nestedMap := valueAs[map[string]string](t, nested[0])
	if nestedMap["scope"] != "core" {
		t.Fatalf("nested scope = %q, want core", nestedMap["scope"])
	}
}

func argumentAs[T any](t *testing.T, arguments map[string]any, name string) T {
	t.Helper()

	return valueAs[T](t, arguments[name])
}

func valueAs[T any](t *testing.T, value any) T {
	t.Helper()

	typed, ok := value.(T)
	if !ok {
		t.Fatalf("value type = %T", value)
	}

	return typed
}
