package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestContinueAgentLoop_rejectsEmptyHistory(t *testing.T) {
	streamCalled := false
	eventCalled := false
	_, err := ContinueAgentLoop(
		context.Background(),
		AgentContext{},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			streamCalled = true
			return AssistantMessage{}, nil
		}},
		func(AgentEvent) error {
			eventCalled = true
			return nil
		},
	)

	var typed *ContinuationError
	if !errors.As(err, &typed) || !errors.Is(err, ErrEmptyContinuationContext) {
		t.Fatalf("ContinueAgentLoop() error = %v, want typed empty-context error", err)
	}
	if streamCalled || eventCalled {
		t.Fatal("ContinueAgentLoop() started work for empty history")
	}
}

func TestContinueAgentLoop_rejectsAssistantTail(t *testing.T) {
	streamCalled := false
	_, err := ContinueAgentLoop(
		context.Background(),
		AgentContext{Messages: []AgentMessage{AssistantMessage{StopReason: StopReasonStop}}},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			streamCalled = true
			return AssistantMessage{}, nil
		}},
		nil,
	)

	if !errors.Is(err, ErrAssistantContinuation) || !errors.Is(err, ErrInvalidContinuation) {
		t.Fatalf("ContinueAgentLoop() error = %v, want assistant-tail error", err)
	}
	if streamCalled {
		t.Fatal("ContinueAgentLoop() called Stream after assistant tail")
	}
}

func TestContinueAgentLoop_usesHistoryWithoutReEmittingIt(t *testing.T) {
	history := []AgentMessage{
		UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{
			ToolCallID: "call-1",
			ToolName:   "qa_search",
			Content:    []ToolResultContent{TextContent{Text: "结果"}},
		},
	}
	original := AgentContext{SystemPrompt: "系统", Messages: history}
	wantOriginal := AgentContext{
		SystemPrompt: "系统",
		Messages: []AgentMessage{
			UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
			AssistantMessage{
				Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
				StopReason: StopReasonToolUse,
			},
			ToolResultMessage{
				ToolCallID: "call-1", ToolName: "qa_search",
				Content: []ToolResultContent{TextContent{Text: "结果"}},
			},
		},
	}
	response := AssistantMessage{
		Content:    []AssistantContent{TextContent{Text: "最终回答"}},
		StopReason: StopReasonStop,
	}
	var modelMessages []AgentMessage
	var messageEnds []AgentMessage

	got, err := ContinueAgentLoop(
		context.Background(),
		original,
		LoopConfig{Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
			modelMessages = modelContext.Messages
			return response, nil
		}},
		func(event AgentEvent) error {
			if end, ok := event.(AgentMessageEndEvent); ok {
				messageEnds = append(messageEnds, end.Message)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ContinueAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, []AgentMessage{response}) {
		t.Fatalf("new messages = %#v, want only response", got)
	}
	if !reflect.DeepEqual(modelMessages, history) {
		t.Fatalf("Model messages = %#v, want full existing history", modelMessages)
	}
	if !reflect.DeepEqual(messageEnds, []AgentMessage{response}) {
		t.Fatalf("message_end events = %#v, want only new response", messageEnds)
	}
	if !reflect.DeepEqual(original, wantOriginal) {
		t.Fatal("ContinueAgentLoop() modified input context")
	}
}
