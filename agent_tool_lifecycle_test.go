package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestRunAgentLoop_wrapsToolLifecycleInTurnEvents(t *testing.T) {
	toolRequest := AssistantMessage{
		Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
		StopReason: StopReasonToolUse,
	}
	finalResponse := AssistantMessage{StopReason: StopReasonStop}
	responses := []AssistantMessage{toolRequest, finalResponse}
	call := 0
	stream := func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
		response := responses[call]
		call++
		return response, nil
	}
	var events []AgentEvent

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{stubTool{}}},
		LoopConfig{Stream: stream},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	wantKinds := []AgentEventKind{
		AgentEventAgentStart, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventToolExecutionStart, AgentEventToolExecutionEnd,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventAgentEnd,
	}
	if gotKinds := agentEventKinds(events); !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
}
