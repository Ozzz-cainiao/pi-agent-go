package agent

import (
	"context"
	"errors"
	"testing"
)

func TestAgent_parallelFailureProjectsFailureAndClearsPendingTools(t *testing.T) {
	tools := []Tool{
		&invalidUpdateTool{name: "invalid"},
		&failureTool{name: "normal"},
	}
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Tools: tools},
		LoopConfig: LoopConfig{Stream: staticAssistantStream(AssistantMessage{
			Content: []AssistantContent{
				ToolCall{ID: "call-1", Name: "invalid"},
				ToolCall{ID: "call-2", Name: "normal"},
			},
			StopReason: StopReasonToolUse,
		})},
	})

	err := agent.Prompt(context.Background())
	if !errors.Is(err, ErrAgentEventSink) {
		t.Fatalf("Prompt() error = %v, want ErrAgentEventSink", err)
	}
	state, stateError := agent.State()
	if stateError != nil || state.IsStreaming || len(state.PendingToolCalls) != 0 {
		t.Fatalf("parallel failure state/error = %#v/%v", state, stateError)
	}
	if tail := assistantMessageAt(t, state.Messages, len(state.Messages)-1); tail.StopReason != StopReasonError {
		t.Fatalf("parallel failure tail = %#v, want error", tail)
	}
}

type invalidUpdateDetails struct {
	values map[string]string
}

type invalidUpdateTool struct{ name string }

func (tool *invalidUpdateTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool *invalidUpdateTool) Execute(
	_ context.Context,
	_ ToolCall,
	onUpdate ToolUpdateFunc,
) (ToolResult, error) {
	onUpdate(ToolResult{Details: invalidUpdateDetails{values: map[string]string{"state": "running"}}})
	return ToolResult{}, nil
}
