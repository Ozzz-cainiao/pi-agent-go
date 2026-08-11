package openairesponses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestProviderStream_accumulatesChunkedTextEvents(t *testing.T) {
	t.Parallel()

	// Given
	fixedTime := time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC)
	stream := strings.Join([]string{
		sseData(t, map[string]any{
			"type":     "response.created",
			"response": map[string]any{"id": "resp_text", "model": "gpt-test", "status": "in_progress"},
		}),
		sseData(t, map[string]any{
			"type": "response.output_text.start", "output_index": 0, "content_index": 0,
		}),
		sseData(t, map[string]any{
			"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "你",
		}),
		sseData(t, map[string]any{
			"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "好",
		}),
		sseData(t, map[string]any{
			"type": "response.output_text.done", "output_index": 0, "content_index": 0, "text": "你好",
		}),
		sseData(t, completedResponse("resp_text", "completed")),
	}, "")
	provider := newStreamTestProvider(t, stream, 7, agent.Clock(func() time.Time { return fixedTime }))
	events := make([]agent.AssistantMessageEvent, 0, 6)

	// When
	message, err := provider.Stream(context.Background(), agent.AgentContext{}, func(event agent.AssistantMessageEvent) error {
		events = append(events, event)
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}
	if got := assistantText(t, message); got != "你好" {
		t.Fatalf("final text = %q, want 你好", got)
	}
	if message.Timestamp != fixedTime.UnixMilli() {
		t.Fatalf("timestamp = %d, want %d", message.Timestamp, fixedTime.UnixMilli())
	}
	assertTextEventSequence(t, events)
}

func TestProviderStream_acceptsLargeTextEventAndNilSink(t *testing.T) {
	t.Parallel()

	// Given
	largeText := strings.Repeat("文", 128*1024)
	stream := strings.Join([]string{
		sseData(t, map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_large"}}),
		sseData(t, map[string]any{"type": "response.output_text.start", "output_index": 0, "content_index": 0}),
		sseData(t, map[string]any{
			"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": largeText,
		}),
		sseData(t, map[string]any{
			"type": "response.output_text.done", "output_index": 0, "content_index": 0, "text": largeText,
		}),
		sseData(t, completedResponse("resp_large", "completed")),
	}, "")
	provider := newStreamTestProvider(t, stream, len(stream), nil)

	// When
	message, err := provider.Stream(context.Background(), agent.AgentContext{}, nil)
	// Then
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}
	if got := assistantText(t, message); got != largeText {
		t.Fatalf("final text length = %d, want %d", len(got), len(largeText))
	}
}

func TestProviderStream_mapsIncompleteMaxOutputTokensToLength(t *testing.T) {
	t.Parallel()

	// Given
	stream := strings.Join([]string{
		sseData(t, map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_short"}}),
		sseData(t, map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "未完"}),
		sseData(t, incompleteResponse("resp_short")),
	}, "")
	provider := newStreamTestProvider(t, stream, 13, nil)
	var done agent.AssistantDoneEvent

	// When
	message, err := provider.Stream(context.Background(), agent.AgentContext{}, func(event agent.AssistantMessageEvent) error {
		if value, ok := event.(agent.AssistantDoneEvent); ok {
			done = value
		}
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}
	if message.StopReason != agent.StopReasonLength || done.Reason != agent.StopReasonLength {
		t.Fatalf("stop reasons = %q/%q, want length", message.StopReason, done.Reason)
	}
	if got := assistantText(t, message); got != "未完" {
		t.Fatalf("final text = %q, want 未完", got)
	}
}

func newStreamTestProvider(t *testing.T, stream string, chunkSize int, clock agent.Clock) *Provider {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		for start := 0; start < len(stream); start += chunkSize {
			end := min(start+chunkSize, len(stream))
			if _, err := writer.Write([]byte(stream[start:end])); err != nil {
				t.Errorf("write stream chunk: %v", err)
				return
			}
			flusher.Flush()
		}
	}))
	t.Cleanup(server.Close)

	provider, err := New(Config{
		APIKey: "test-key", BaseURL: server.URL, Model: "gpt-test", Clock: clock,
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	return provider
}

func sseData(t *testing.T, payload any) string {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal SSE payload: %v", err)
	}
	return "data: " + string(data) + "\n\n"
}

func completedResponse(id, status string) map[string]any {
	return map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": id, "model": "gpt-test", "status": status,
			"usage": map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
		},
	}
}

func incompleteResponse(id string) map[string]any {
	return map[string]any{
		"type": "response.incomplete",
		"response": map[string]any{
			"id": id, "model": "gpt-test", "status": "incomplete",
			"usage":              map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
			"incomplete_details": map[string]any{"reason": "max_output_tokens"},
		},
	}
}

func assistantText(t *testing.T, message agent.AssistantMessage) string {
	t.Helper()

	if len(message.Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(message.Content))
	}
	text, ok := message.Content[0].(agent.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want TextContent", message.Content[0])
	}
	return text.Text
}

func assertTextEventSequence(t *testing.T, events []agent.AssistantMessageEvent) {
	t.Helper()

	if len(events) != 6 {
		t.Fatalf("event count = %d, want 6", len(events))
	}
	_ = eventAs[agent.AssistantStartEvent](t, events[0])
	_ = eventAs[agent.AssistantTextStartEvent](t, events[1])
	firstDelta := eventAs[agent.AssistantTextDeltaEvent](t, events[2])
	if firstDelta.Delta != "你" || assistantText(t, firstDelta.Partial) != "你" {
		t.Fatalf("event[2] = %#v, want first text delta", events[2])
	}
	secondDelta := eventAs[agent.AssistantTextDeltaEvent](t, events[3])
	if secondDelta.Delta != "好" || assistantText(t, secondDelta.Partial) != "你好" {
		t.Fatalf("event[3] = %#v, want second text delta", events[3])
	}
	textEnd := eventAs[agent.AssistantTextEndEvent](t, events[4])
	if textEnd.Content != "你好" || assistantText(t, textEnd.Partial) != "你好" {
		t.Fatalf("event[4] = %T, want AssistantTextEndEvent", events[4])
	}
	done := eventAs[agent.AssistantDoneEvent](t, events[5])
	if done.Reason != agent.StopReasonStop || assistantText(t, done.Message) != "你好" {
		t.Fatalf("event[5] = %T, want AssistantDoneEvent", events[5])
	}
}

func eventAs[T agent.AssistantMessageEvent](t *testing.T, event agent.AssistantMessageEvent) T {
	t.Helper()

	value, ok := event.(T)
	if !ok {
		t.Fatalf("event = %T, want requested Assistant event", event)
	}
	return value
}
