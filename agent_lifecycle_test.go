package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunAgentLoop_emitsSimpleLifecycleInOrder(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "你好"}}}
	response := AssistantMessage{
		Content:    []AssistantContent{TextContent{Text: "你好"}},
		StopReason: StopReasonStop,
	}
	var events []AgentEvent

	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{},
		LoopConfig{Stream: staticAssistantStream(response)},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	wantKinds := []AgentEventKind{
		AgentEventAgentStart,
		AgentEventTurnStart,
		AgentEventMessageStart,
		AgentEventMessageEnd,
		AgentEventMessageStart,
		AgentEventMessageEnd,
		AgentEventTurnEnd,
		AgentEventAgentEnd,
	}
	if gotKinds := agentEventKinds(events); !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
	end, ok := events[len(events)-1].(AgentEndEvent)
	if !ok || !reflect.DeepEqual(end.Messages, got) {
		t.Fatalf("agent_end = %#v, want returned messages %#v", end, got)
	}
}

func TestRunAgentLoop_closesLifecycleWhenStreamFails(t *testing.T) {
	streamError := errors.New("stream failed")
	var events []AgentEvent
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			return AssistantMessage{}, streamError
		}},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)

	if !errors.Is(err, streamError) {
		t.Fatalf("RunAgentLoop() error = %v, want stream error", err)
	}
	wantKinds := []AgentEventKind{AgentEventAgentStart, AgentEventTurnStart, AgentEventAgentEnd}
	if gotKinds := agentEventKinds(events); !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
}

func TestRunAgentLoop_returnsTypedAgentEventSinkError(t *testing.T) {
	sinkError := errors.New("sink failed")
	streamCalled := false
	var events []AgentEventKind
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			streamCalled = true
			return AssistantMessage{}, nil
		}},
		func(event AgentEvent) error {
			events = append(events, event.Kind())
			if event.Kind() == AgentEventTurnStart {
				return sinkError
			}
			return nil
		},
	)

	var typed *AgentEventSinkError
	if !errors.As(err, &typed) || !errors.Is(err, ErrAgentEventSink) || !errors.Is(err, sinkError) {
		t.Fatalf("RunAgentLoop() error = %v, want typed sink error", err)
	}
	if streamCalled {
		t.Fatal("StreamFunc called after AgentEventSink failure")
	}
	wantEvents := []AgentEventKind{
		AgentEventAgentStart,
		AgentEventTurnStart,
		AgentEventAgentEnd,
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("event kinds = %#v, want %#v", events, wantEvents)
	}
}

func TestRunAgentLoop_sendsDefensiveMessageSnapshots(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "原始内容"}}}
	stream := func(_ context.Context, context AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
		user, ok := context.Messages[0].(UserMessage)
		if !ok {
			t.Fatalf("Model context message type = %T, want UserMessage", context.Messages[0])
		}
		text, ok := user.Content[0].(TextContent)
		if !ok {
			t.Fatalf("Model context content type = %T, want TextContent", user.Content[0])
		}
		if text.Text != "原始内容" {
			t.Fatalf("Model context text = %q, want original snapshot", text.Text)
		}
		return AssistantMessage{StopReason: StopReasonStop}, nil
	}

	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{},
		LoopConfig{Stream: stream},
		func(event AgentEvent) error {
			start, ok := event.(AgentMessageStartEvent)
			if !ok {
				return nil
			}
			user, ok := start.Message.(UserMessage)
			if ok {
				user.Content[0] = TextContent{Text: "已篡改"}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	returnedMessage, ok := got[0].(UserMessage)
	if !ok {
		t.Fatalf("returned message type = %T, want UserMessage", got[0])
	}
	returned, ok := returnedMessage.Content[0].(TextContent)
	if !ok {
		t.Fatalf("returned content type = %T, want TextContent", returnedMessage.Content[0])
	}
	if returned.Text != "原始内容" {
		t.Fatalf("returned text = %q, want original snapshot", returned.Text)
	}
}

func staticAssistantStream(message AssistantMessage) StreamFunc {
	return func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
		return message, nil
	}
}

func agentEventKinds(events []AgentEvent) []AgentEventKind {
	kinds := make([]AgentEventKind, len(events))
	for index, event := range events {
		kinds[index] = event.Kind()
	}
	return kinds
}
