package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunAgentLoop_callsModelWithHistoryAndPrompt(t *testing.T) {
	// 准备
	history := UserMessage{
		Content: []UserContent{
			TextContent{Text: "历史问题"},
		},
		Timestamp: 1,
	}
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "本轮问题"},
		},
		Timestamp: 2,
	}
	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模型回答"},
		},
		StopReason: StopReasonStop,
		Timestamp:  3,
	}

	initial := AgentContext{
		SystemPrompt: "你是一个助手。",
		Messages:     []Message{history},
		Tools:        []Tool{stubTool{}},
	}

	var receivedContext AgentContext
	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		receivedContext = agentContext

		return response, nil
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]Message{prompt},
		initial,
		streamFn,
		nil,
	)

	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if len(initial.Messages) != 1 {
		t.Fatalf("initial message count = %d, want 1", len(initial.Messages))
	}
	if len(receivedContext.Messages) != 2 {
		t.Fatalf(
			"model context message count = %d, want 2",
			len(receivedContext.Messages),
		)
	}

	want := []Message{prompt, response}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunAgentLoop() = %#v, want %#v", got, want)
	}
}

func TestRunAgentLoop_doesNotCallModelWhenContextCanceled(t *testing.T) {
	// 准备
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "本轮问题"},
		},
		Timestamp: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	modelCalled := false
	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalled = true

		return AssistantMessage{}, nil
	})

	// 执行
	got, err := RunAgentLoop(
		ctx,
		[]Message{prompt},
		AgentContext{},
		streamFn,
		nil,
	)

	// 验证
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAgentLoop() error = %v, want context.Canceled", err)
	}
	if modelCalled {
		t.Fatal("RunAgentLoop() called Model after context cancellation")
	}

	want := []Message{prompt}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunAgentLoop() = %#v, want %#v", got, want)
	}
}
func TestRunAgentLoop_forwardsAssistantEvents(t *testing.T) {
	// 准备
	partial := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模"},
		},
		StopReason: StopReasonPending,
	}

	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模型回答"},
		},
		StopReason: StopReasonStop,
	}

	wantEvents := []AssistantMessageEvent{
		AssistantStartEvent{
			Partial: AssistantMessage{
				StopReason: StopReasonPending,
			},
		},
		AssistantTextDeltaEvent{
			ContentIndex: 0,
			Delta:        "模",
			Partial:      partial,
		},
	}

	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		emit AssistantMessageEventSink,
	) (AssistantMessage, error) {
		for _, event := range wantEvents {
			if err := emit(event); err != nil {
				return AssistantMessage{}, err
			}
		}

		return response, nil
	})

	var gotEvents []AssistantMessageEvent
	emit := AssistantMessageEventSink(func(event AssistantMessageEvent) error {
		gotEvents = append(gotEvents, event)

		return nil
	})

	// 执行
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		streamFn,
		emit,
	)

	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("events = %#v, want %#v", gotEvents, wantEvents)
	}
}

func TestRunAgentLoop_executesToolCallAndContinuesModel(t *testing.T) {
	// 准备
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "查询 Agent Loop"},
		},
		Timestamp: 1,
	}

	toolRequest := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "Agent Loop",
				},
			},
		},
		StopReason: StopReasonToolUse,
		Timestamp:  2,
	}

	finalResponse := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "Agent Loop 负责协调模型和工具。"},
		},
		StopReason: StopReasonStop,
		Timestamp:  4,
	}

	modelCalls := 0
	var secondContext AgentContext

	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++

		switch modelCalls {
		case 1:
			return toolRequest, nil
		case 2:
			secondContext = agentContext

			return finalResponse, nil
		default:
			return AssistantMessage{}, errors.New("unexpected Model call")
		}
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]Message{prompt},
		AgentContext{
			Tools: []Tool{stubTool{}},
		},
		streamFn,
		nil,
	)

	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("Model call count = %d, want 2", modelCalls)
	}
	if len(secondContext.Messages) != 3 {
		t.Fatalf(
			"second Model context message count = %d, want 3",
			len(secondContext.Messages),
		)
	}
	if len(got) != 4 {
		t.Fatalf("new message count = %d, want 4", len(got))
	}

	toolResult, ok := got[2].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[2] type = %T, want ToolResultMessage", got[2])
	}
	if toolResult.ToolCallID != "call-1" {
		t.Fatalf(
			"ToolCallID = %q, want %q",
			toolResult.ToolCallID,
			"call-1",
		)
	}
	if toolResult.IsError {
		t.Fatal("tool result IsError = true, want false")
	}
	if toolResult.Timestamp <= 0 {
		t.Fatalf("tool result Timestamp = %d, want positive value", toolResult.Timestamp)
	}
	if !reflect.DeepEqual(got[3], finalResponse) {
		t.Fatalf("final message = %#v, want %#v", got[3], finalResponse)
	}
}
