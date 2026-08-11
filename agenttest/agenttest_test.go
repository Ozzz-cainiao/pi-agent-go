package agenttest

import (
	"context"
	"errors"
	"testing"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestScriptedStream_returnsStepsAndCapturesContexts(t *testing.T) {
	t.Parallel()

	// Given
	want := agent.AssistantMessage{
		Content: []agent.AssistantContent{agent.TextContent{Text: "完成"}}, StopReason: agent.StopReasonStop,
	}
	stream := NewScriptedStream(StreamStep{Response: want})
	modelContext := agent.AgentContext{SystemPrompt: "system"}

	// When
	got, err := stream.Stream(context.Background(), modelContext, nil)

	// Then
	if err != nil || got.StopReason != agent.StopReasonStop {
		t.Fatalf("Stream() = %#v/%v, want scripted response", got, err)
	}
	calls := stream.Calls()
	if len(calls) != 1 || calls[0].SystemPrompt != "system" {
		t.Fatalf("Calls() = %#v, want captured context", calls)
	}
	_, err = stream.Stream(context.Background(), agent.AgentContext{}, nil)
	if !errors.Is(err, ErrScriptExhausted) {
		t.Fatalf("exhausted Stream() error = %v, want ErrScriptExhausted", err)
	}
}

func TestScriptedTool_recordsCallsAndUsesHandler(t *testing.T) {
	t.Parallel()

	// Given
	tool := NewScriptedTool(agent.ToolDefinition{Name: "search"}, func(
		_ context.Context,
		call agent.ToolCall,
		_ agent.ToolUpdateFunc,
	) (agent.ToolResult, error) {
		return agent.ToolResult{Content: []agent.ToolResultContent{agent.TextContent{Text: call.Name}}}, nil
	})
	call := agent.ToolCall{ID: "call-1", Name: "search", Arguments: map[string]any{"query": "pi"}}

	// When
	result, err := tool.Execute(context.Background(), call, nil)

	// Then
	if err != nil || len(result.Content) != 1 {
		t.Fatalf("Execute() = %#v/%v", result, err)
	}
	calls := tool.Calls()
	if len(calls) != 1 || calls[0].ID != "call-1" {
		t.Fatalf("Calls() = %#v, want recorded call", calls)
	}
}

func TestClock_advancesDeterministically(t *testing.T) {
	t.Parallel()

	// Given
	start := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	clock := Clock(start, time.Second)

	// When
	first, second := clock(), clock()

	// Then
	if !first.Equal(start) || !second.Equal(start.Add(time.Second)) {
		t.Fatalf("clock values = %v/%v", first, second)
	}
}
