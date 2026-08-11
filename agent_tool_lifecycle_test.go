package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

var errNilToolUpdate = errors.New("tool update callback is nil")

type updatingTool struct{}

func (updatingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "qa_search"}
}

func (updatingTool) Execute(
	_ context.Context,
	_ ToolCall,
	onUpdate ToolUpdateFunc,
) (ToolResult, error) {
	if onUpdate == nil {
		return ToolResult{}, errNilToolUpdate
	}
	onUpdate(ToolResult{
		Content: []ToolResultContent{TextContent{Text: "正在检索"}},
		Details: map[string]any{"progress": 0.5},
	})
	return ToolResult{
		Content: []ToolResultContent{TextContent{Text: "检索完成"}},
	}, nil
}

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
		AgentContext{Tools: []Tool{updatingTool{}}},
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
		AgentEventToolExecutionStart, AgentEventToolExecutionUpdate,
		AgentEventToolExecutionEnd,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventAgentEnd,
	}
	if gotKinds := agentEventKinds(events); !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
	update, ok := events[5].(AgentToolExecutionUpdateEvent)
	if !ok {
		t.Fatalf("event[5] type = %T, want AgentToolExecutionUpdateEvent", events[5])
	}
	if update.ToolCallID != "call-1" || update.ToolName != "qa_search" {
		t.Fatalf("tool update identity = %#v, want call-1/qa_search", update)
	}
	wantPartial := []ToolResultContent{TextContent{Text: "正在检索"}}
	if !reflect.DeepEqual(update.PartialResult.Content, wantPartial) {
		t.Fatalf("partial result = %#v, want %#v", update.PartialResult.Content, wantPartial)
	}
}
