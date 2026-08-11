package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAgent_ContinueUsesExistingHistoryWithoutReEmittingIt(t *testing.T) {
	history := []AgentMessage{
		UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{ToolCallID: "call-1", ToolName: "search"},
	}
	response := AssistantMessage{
		Content:    []AssistantContent{TextContent{Text: "回答"}},
		StopReason: StopReasonStop,
	}
	var modelMessages []AgentMessage
	agent, err := NewAgent(AgentOptions{
		InitialState: &AgentInitialState{Messages: history},
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelMessages = modelContext.Messages
			return response, nil
		}},
	})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	var messageEnds []AgentMessage
	_, err = agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if end, ok := event.(AgentMessageEndEvent); ok {
			messageEnds = append(messageEnds, end.Message)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}
	before, err := agent.State()
	if err != nil {
		t.Fatalf("State() before Continue returned error: %v", err)
	}

	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if !reflect.DeepEqual(modelMessages, before.Messages) || !reflect.DeepEqual(messageEnds, []AgentMessage{response}) {
		t.Fatalf("Model/events = %#v/%#v, want history/new response", modelMessages, messageEnds)
	}
	state, err := agent.State()
	if err != nil || len(state.Messages) != len(history)+1 {
		t.Fatalf("continued state/error = %#v/%v", state, err)
	}
}

func TestAgent_failedContinueRestoresIdleState(t *testing.T) {
	agent := newRunControlAgent(t, staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}))
	err := agent.Continue(context.Background())
	if !errors.Is(err, ErrEmptyContinuationContext) {
		t.Fatalf("Continue() error = %v, want ErrEmptyContinuationContext", err)
	}
	state, stateError := agent.State()
	if stateError != nil || state.IsStreaming || len(state.Messages) != 1 {
		t.Fatalf("state after failed Continue = %#v/%v", state, stateError)
	}
	if tail := assistantMessageAt(t, state.Messages, 0); tail.StopReason != StopReasonError {
		t.Fatalf("failed Continue tail = %#v, want error", tail)
	}
}
