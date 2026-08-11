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
		Messages:     []AgentMessage{history},
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
		[]AgentMessage{prompt},
		initial,
		LoopConfig{Stream: streamFn},
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

	want := []AgentMessage{prompt, response}
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
		[]AgentMessage{prompt},
		AgentContext{},
		LoopConfig{Stream: streamFn},
		nil,
	)

	// 验证
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAgentLoop() error = %v, want context.Canceled", err)
	}
	if modelCalled {
		t.Fatal("RunAgentLoop() called Model after context cancellation")
	}

	want := []AgentMessage{prompt}
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
		LoopConfig{Stream: streamFn},
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
