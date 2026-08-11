package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunAgentLoop_handlesEveryStopReason(t *testing.T) {
	tests := []struct {
		name       string
		reason     StopReason
		withTool   bool
		modelCalls int
		toolCalls  int
	}{
		{name: "pending", reason: StopReasonPending, modelCalls: 1},
		{name: "stop", reason: StopReasonStop, modelCalls: 1},
		{name: "length", reason: StopReasonLength, modelCalls: 1},
		{name: "toolUse", reason: StopReasonToolUse, withTool: true, modelCalls: 2, toolCalls: 1},
		{name: "error", reason: StopReasonError, withTool: true, modelCalls: 1},
		{name: "aborted", reason: StopReasonAborted, withTool: true, modelCalls: 1},
		{name: "deferred", reason: StopReasonDeferred, modelCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := &countingTool{}
			calls := 0
			stream := func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				calls++
				if calls > 1 {
					return AssistantMessage{StopReason: StopReasonStop}, nil
				}
				response := AssistantMessage{StopReason: test.reason}
				if test.withTool {
					response.Content = []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}}
				}
				return response, nil
			}

			_, err := RunAgentLoop(context.Background(), nil, AgentContext{Tools: []Tool{tool}}, LoopConfig{Stream: stream}, nil)
			if err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
			if calls != test.modelCalls || tool.calls != test.toolCalls {
				t.Fatalf("calls = model:%d tool:%d, want %d/%d", calls, tool.calls, test.modelCalls, test.toolCalls)
			}
		})
	}
}

func TestRunAgentLoop_returnsTypedErrorAtConfiguredMaxTurns(t *testing.T) {
	modelCalls := 0
	tool := &countingTool{}
	var events []AgentEvent
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{
			MaxTurns: 3,
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{
					Content:    []AssistantContent{ToolCall{ID: "call", Name: "qa_search"}},
					StopReason: StopReasonToolUse,
				}, nil
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)

	var turnError *MaxTurnsError
	if !errors.As(err, &turnError) || !errors.Is(err, ErrMaxTurnsExceeded) {
		t.Fatalf("RunAgentLoop() error = %v, want typed max turns error", err)
	}
	if turnError.MaxTurns != 3 || modelCalls != 3 || tool.calls != 3 {
		t.Fatalf("limit/calls = %d/%d/%d, want 3/3/3", turnError.MaxTurns, modelCalls, tool.calls)
	}
	if countEventKind(events, AgentEventTurnStart) != 3 || countEventKind(events, AgentEventTurnEnd) != 3 {
		t.Fatalf("turn events = start:%d end:%d, want 3/3", countEventKind(events, AgentEventTurnStart), countEventKind(events, AgentEventTurnEnd))
	}
}

func TestRunAgentLoop_shouldStopAfterCompletedTurn(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "开始"}}}
	tool := &countingTool{}
	modelCalls := 0
	steeringPolls := 0
	followUpPolls := 0
	var callback ShouldStopAfterTurnContext
	var events []AgentEvent

	messages, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{
					Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
					StopReason: StopReasonToolUse,
				}, nil
			},
			ShouldStopAfterTurn: func(_ context.Context, turn ShouldStopAfterTurnContext) (bool, error) {
				callback = turn
				return true, nil
			},
			GetSteeringMessages: func(context.Context) ([]AgentMessage, error) {
				steeringPolls++
				return nil, nil
			},
			GetFollowUpMessages: func(context.Context) ([]AgentMessage, error) {
				followUpPolls++
				return []AgentMessage{prompt}, nil
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 1 || tool.calls != 1 || steeringPolls != 1 || followUpPolls != 0 {
		t.Fatalf("calls = model:%d tool:%d steering:%d follow-up:%d, want 1/1/1/0", modelCalls, tool.calls, steeringPolls, followUpPolls)
	}
	if len(callback.ToolResults) != 1 || len(callback.Context.Messages) != 3 || !reflect.DeepEqual(callback.NewMessages, messages) {
		t.Fatalf("stop callback snapshot = %#v, messages = %#v", callback, messages)
	}
	wantTail := []AgentEventKind{AgentEventToolExecutionEnd, AgentEventMessageStart, AgentEventMessageEnd, AgentEventTurnEnd, AgentEventAgentEnd}
	gotKinds := agentEventKinds(events)
	if !reflect.DeepEqual(gotKinds[len(gotKinds)-len(wantTail):], wantTail) {
		t.Fatalf("event tail = %#v, want %#v", gotKinds, wantTail)
	}
}

func TestRunAgentLoop_returnsTypedTurnControlHookError(t *testing.T) {
	hookError := errors.New("stop hook failed")
	var events []AgentEvent
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{
			Stream: staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}),
			ShouldStopAfterTurn: func(context.Context, ShouldStopAfterTurnContext) (bool, error) {
				return false, hookError
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)

	var typed *TurnControlHookError
	if !errors.As(err, &typed) || !errors.Is(err, ErrTurnControlHook) || !errors.Is(err, hookError) {
		t.Fatalf("RunAgentLoop() error = %v, want typed hook error", err)
	}
	if typed.Hook != "ShouldStopAfterTurn" {
		t.Fatalf("hook name = %q, want ShouldStopAfterTurn", typed.Hook)
	}
	wantTail := []AgentEventKind{AgentEventTurnEnd, AgentEventAgentEnd}
	gotKinds := agentEventKinds(events)
	if !reflect.DeepEqual(gotKinds[len(gotKinds)-len(wantTail):], wantTail) {
		t.Fatalf("event kinds = %#v, want closed lifecycle", gotKinds)
	}
}

func countEventKind(events []AgentEvent, kind AgentEventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind() == kind {
			count++
		}
	}
	return count
}
