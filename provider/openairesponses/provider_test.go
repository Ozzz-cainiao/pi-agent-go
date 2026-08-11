package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestProviderStream_sendsResponsesRequest(t *testing.T) {
	t.Parallel()

	requestReceived := make(chan requestBody, 1)
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/responses" {
			t.Errorf("request path = %q, want /responses", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}

		var body requestBody
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}
		requestReceived <- body

		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte(
			"event: response.completed\n" +
				`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}` +
				"\n\n",
		))
	}))
	defer server.Close()

	provider, err := New(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	message, err := provider.Stream(context.Background(), agent.AgentContext{
		SystemPrompt: "你是测试助手。",
		Messages: []agent.Message{
			agent.UserMessage{Content: []agent.UserContent{
				agent.TextContent{Text: "你好"},
			}},
		},
		Tools: []agent.Tool{stubTool{}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() returned error: %v", err)
	}

	body := <-requestReceived
	if body.Model != "gpt-test" || !body.Stream {
		t.Fatalf("request model/stream = %q/%v, want gpt-test/true", body.Model, body.Stream)
	}
	if body.Instructions != "你是测试助手。" {
		t.Errorf("instructions = %q", body.Instructions)
	}
	if len(body.Input) != 1 || body.Input[0].Role != "user" {
		t.Fatalf("input = %#v, want one user message", body.Input)
	}
	if got := body.Input[0].Content[0].Text; got != "你好" {
		t.Errorf("input text = %q, want 你好", got)
	}
	if len(body.Tools) != 1 || body.Tools[0].Name != "search" {
		t.Fatalf("tools = %#v, want search", body.Tools)
	}
	if message.ResponseID != "resp_1" || message.Model != "gpt-test" {
		t.Errorf("response identity = %q/%q", message.ResponseID, message.Model)
	}
	if message.StopReason != agent.StopReasonStop {
		t.Errorf("stop reason = %q, want stop", message.StopReason)
	}
	if message.Usage.TotalTokens != 5 {
		t.Errorf("total tokens = %d, want 5", message.Usage.TotalTokens)
	}
}

func TestProviderStream_returnsTypedAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	provider, err := New(Config{
		APIKey:  "bad-key",
		BaseURL: server.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	_, err = provider.Stream(context.Background(), agent.AgentContext{}, nil)
	if !errors.Is(err, ErrAPIResponse) {
		t.Fatalf("Stream() error = %v, want ErrAPIResponse", err)
	}
}

type stubTool struct{}

func (stubTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "search",
		Description: "搜索资料",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func (stubTool) Execute(
	context.Context,
	agent.ToolCall,
	agent.ToolUpdateFunc,
) (agent.ToolResult, error) {
	return agent.ToolResult{}, nil
}
