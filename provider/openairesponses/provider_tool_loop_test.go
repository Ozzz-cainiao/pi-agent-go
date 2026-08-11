package openairesponses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestProviderStream_completesModelToolModelLoop(t *testing.T) {
	t.Parallel()

	// Given
	requests := make(chan requestBody, 2)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body requestBody
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- body
		writer.Header().Set("Content-Type", "text/event-stream")
		stream := toolCallResponseStream(t)
		if requestCount.Add(1) == 2 {
			stream = finalTextResponseStream(t)
		}
		if _, err := writer.Write([]byte(stream)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	toolCalls := make(chan agent.ToolCall, 1)
	provider, err := New(Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-test"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	var toolEvents []string

	// When
	messages, err := agent.RunAgentLoop(
		context.Background(),
		[]agent.AgentMessage{agent.UserMessage{Content: []agent.UserContent{agent.TextContent{Text: "查询 Agent"}}}},
		agent.AgentContext{Tools: []agent.Tool{providerSearchTool{calls: toolCalls}}},
		agent.LoopConfig{Stream: provider.Stream},
		func(event agent.AgentEvent) error {
			update, ok := event.(agent.AgentMessageUpdateEvent)
			if !ok {
				return nil
			}
			switch update.AssistantEvent.(type) {
			case agent.AssistantToolCallStartEvent:
				toolEvents = append(toolEvents, "start")
			case agent.AssistantToolCallDeltaEvent:
				toolEvents = append(toolEvents, "delta")
			case agent.AssistantToolCallEndEvent:
				toolEvents = append(toolEvents, "end")
			}
			return nil
		},
	)
	// Then
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requestCount.Load())
	}
	if !reflect.DeepEqual(toolEvents, []string{"start", "delta", "delta", "end"}) {
		t.Fatalf("tool events = %#v, want start/delta/delta/end", toolEvents)
	}
	assertProviderToolMessages(t, messages)
	assertSecondToolRequest(t, <-requests, <-requests)
	call := <-toolCalls
	if call.Name != "qa_search" || call.Arguments["query"] != "Agent" {
		t.Fatalf("tool call = %#v, want qa_search query Agent", call)
	}
}

func toolCallResponseStream(t *testing.T) string {
	t.Helper()

	return sseData(t, map[string]any{
		"type": "response.created", "response": map[string]any{"id": "resp_tool", "model": "gpt-test"},
	}) + sseData(t, map[string]any{
		"type": "response.output_item.added", "output_index": 0,
		"item": map[string]any{"type": "function_call", "call_id": "call_1", "name": "qa_search", "arguments": ""},
	}) + sseData(t, map[string]any{
		"type": "response.function_call_arguments.delta", "output_index": 0, "delta": `{"query":"`,
	}) + sseData(t, map[string]any{
		"type": "response.function_call_arguments.delta", "output_index": 0, "delta": `Agent"}`,
	}) + sseData(t, map[string]any{
		"type": "response.function_call_arguments.done", "output_index": 0, "arguments": `{"query":"Agent"}`,
	}) + sseData(t, map[string]any{
		"type": "response.output_item.done", "output_index": 0,
		"item": map[string]any{
			"type": "function_call", "call_id": "call_1", "name": "qa_search", "arguments": `{"query":"Agent"}`,
		},
	}) + sseData(t, map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp_tool", "model": "gpt-test", "status": "completed",
			"usage": map[string]any{
				"input_tokens": 10, "output_tokens": 6, "total_tokens": 16,
				"input_tokens_details":  map[string]any{"cached_tokens": 3, "cache_write_tokens": 2},
				"output_tokens_details": map[string]any{"reasoning_tokens": 4},
			},
		},
	})
}

func finalTextResponseStream(t *testing.T) string {
	t.Helper()

	return sseData(t, map[string]any{
		"type": "response.created", "response": map[string]any{"id": "resp_final", "model": "gpt-test"},
	}) + sseData(t, map[string]any{
		"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "查询完成",
	}) + sseData(t, map[string]any{
		"type": "response.output_text.done", "output_index": 0, "content_index": 0, "text": "查询完成",
	}) + sseData(t, completedResponse("resp_final", "completed"))
}

func assertProviderToolMessages(t *testing.T, messages []agent.AgentMessage) {
	t.Helper()

	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	toolMessage, ok := messages[1].(agent.AssistantMessage)
	if !ok || toolMessage.StopReason != agent.StopReasonToolUse {
		t.Fatalf("message[1] = %#v, want ToolUse AssistantMessage", messages[1])
	}
	call, ok := toolMessage.Content[0].(agent.ToolCall)
	if !ok || call.ID != "call_1" || call.Arguments["query"] != "Agent" {
		t.Fatalf("assistant tool call = %#v, want parsed qa_search call", toolMessage.Content)
	}
	if toolMessage.Usage.InputTokens != 5 || toolMessage.Usage.CacheReadTokens != 3 ||
		toolMessage.Usage.CacheWriteTokens != 2 || toolMessage.Usage.ReasoningTokens == nil ||
		*toolMessage.Usage.ReasoningTokens != 4 {
		t.Fatalf("assistant usage = %#v, want cache and reasoning details", toolMessage.Usage)
	}
	finalMessage, ok := messages[3].(agent.AssistantMessage)
	if !ok || assistantText(t, finalMessage) != "查询完成" {
		t.Fatalf("message[3] = %#v, want final text", messages[3])
	}
}

func assertSecondToolRequest(t *testing.T, _ requestBody, second requestBody) {
	t.Helper()

	var functionCall, functionOutput *requestInputItem
	for index := range second.Input {
		item := &second.Input[index]
		switch item.Type {
		case "function_call":
			functionCall = item
		case "function_call_output":
			functionOutput = item
		}
	}
	if functionCall == nil || functionCall.CallID != "call_1" || functionCall.Arguments != `{"query":"Agent"}` {
		t.Fatalf("function call input = %#v", functionCall)
	}
	if functionOutput == nil || functionOutput.CallID != "call_1" || functionOutput.Output != "找到 Agent" {
		t.Fatalf("function output input = %#v", functionOutput)
	}
}

type providerSearchTool struct {
	calls chan<- agent.ToolCall
}

func (tool providerSearchTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name: "qa_search", Parameters: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}}}`),
	}
}

func (tool providerSearchTool) Execute(
	_ context.Context,
	call agent.ToolCall,
	_ agent.ToolUpdateFunc,
) (agent.ToolResult, error) {
	tool.calls <- call
	return agent.ToolResult{Content: []agent.ToolResultContent{agent.TextContent{Text: "找到 Agent"}}}, nil
}
