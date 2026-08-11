package openairesponses

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestProviderStream_rejectsNonObjectToolArguments(t *testing.T) {
	t.Parallel()

	// Given
	stream := sseData(t, map[string]any{
		"type": "response.output_item.added", "output_index": 0,
		"item": map[string]any{"type": "function_call", "call_id": "call_1", "name": "search"},
	}) + sseData(t, map[string]any{
		"type": "response.function_call_arguments.done", "output_index": 0, "arguments": `["not","object"]`,
	}) + sseData(t, completedResponse("resp_1", "completed"))
	provider := newStreamTestProvider(t, stream, len(stream), nil)

	// When
	_, err := provider.Stream(context.Background(), agent.AgentContext{}, nil)

	// Then
	assertProtocolError(t, err)
}

func TestProviderStream_returnsTypedFailedResponse(t *testing.T) {
	t.Parallel()

	// Given
	stream := sseData(t, map[string]any{
		"type": "response.created", "response": map[string]any{"id": "resp_failed"},
	}) + sseData(t, map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id": "resp_failed", "status": "failed",
			"error": map[string]any{"code": "server_error", "message": "provider failed"},
		},
	})
	provider := newStreamTestProvider(t, stream, 11, nil)
	var errorEvent agent.AssistantErrorEvent

	// When
	_, err := provider.Stream(context.Background(), agent.AgentContext{}, func(event agent.AssistantMessageEvent) error {
		if typed, ok := event.(agent.AssistantErrorEvent); ok {
			errorEvent = typed
		}
		return nil
	})

	// Then
	if !errors.Is(err, ErrResponseFailed) {
		t.Fatalf("Stream() error = %v, want ErrResponseFailed", err)
	}
	var failedError *ResponseFailedError
	if !errors.As(err, &failedError) || failedError.Code != "server_error" {
		t.Fatalf("Stream() error = %#v, want typed server_error", err)
	}
	if errorEvent.Reason != agent.StopReasonError || errorEvent.Error.ErrorMessage != "provider failed" {
		t.Fatalf("error event = %#v, want failed Assistant lifecycle", errorEvent)
	}
}

func TestProviderStream_returnsTypedHTTPError(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		if _, err := writer.Write([]byte(`{"error":{"code":"invalid_api_key","message":"bad key"}}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	provider, err := New(Config{APIKey: "bad-key", BaseURL: server.URL, Model: "gpt-test"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// When
	_, err = provider.Stream(context.Background(), agent.AgentContext{}, nil)

	// Then
	if !errors.Is(err, ErrAPIResponse) {
		t.Fatalf("Stream() error = %v, want ErrAPIResponse", err)
	}
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusUnauthorized || apiError.Code != "invalid_api_key" {
		t.Fatalf("Stream() error = %#v, want typed 401 APIError", err)
	}
}

func TestProviderStream_cancelsRequestAndClosesBodyRead(t *testing.T) {
	t.Parallel()

	// Given
	requestStarted := make(chan struct{})
	requestClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		writer.WriteHeader(http.StatusOK)
		flusher.Flush()
		close(requestStarted)
		<-request.Context().Done()
		close(requestClosed)
	}))
	defer server.Close()
	provider, err := New(Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-test"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, streamErr := provider.Stream(ctx, agent.AgentContext{}, nil)
		result <- streamErr
	}()
	<-requestStarted

	// When
	cancel()

	// Then
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream() error = %v, want context.Canceled", err)
	}
	<-requestClosed
}
