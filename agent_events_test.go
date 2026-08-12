package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
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

func TestAgent_SubscribeDeliversLifecycleSeriallyAndUnsubscribes(t *testing.T) {
	agent := newSubscriptionTestAgent(t)
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "你好"}}}
	var calls []string

	unsubscribeFirst, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		calls = append(calls, "first:"+string(event.Kind()))
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(first) returned error: %v", err)
	}
	_, err = agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		calls = append(calls, "second:"+string(event.Kind()))
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(second) returned error: %v", err)
	}

	if err := agent.Prompt(context.Background(), prompt); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	wantKinds := []AgentEventKind{
		AgentEventAgentStart, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventAgentEnd,
	}
	wantCalls := make([]string, 0, len(wantKinds)*2)
	for _, kind := range wantKinds {
		wantCalls = append(wantCalls, "first:"+string(kind), "second:"+string(kind))
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("subscriber calls = %#v, want %#v", calls, wantCalls)
	}

	unsubscribeFirst()
	calls = nil
	if err := agent.Prompt(context.Background(), prompt); err != nil {
		t.Fatalf("second Prompt() returned error: %v", err)
	}
	for _, call := range calls {
		if call[:7] != "second:" {
			t.Fatalf("call after unsubscribe = %q, want only second subscriber", call)
		}
	}
}

func TestAgent_PromptPassesContextAndUpdatesStateBeforeSubscriber(t *testing.T) {
	agent := newSubscriptionTestAgent(t)
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "current-run")
	observed := ""

	_, err := agent.Subscribe(func(subscriberContext context.Context, event AgentEvent) error {
		if event.Kind() != AgentEventMessageEnd {
			return nil
		}
		if subscriberContext.Value(contextKey{}) != "current-run" {
			return errors.New("subscriber received the wrong context")
		}
		state, stateError := agent.State()
		if stateError != nil {
			return stateError
		}
		observed = fmt.Sprintf("%d", len(state.Messages))
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}

	prompt := UserMessage{Content: []UserContent{TextContent{Text: "你好"}}}
	if err := agent.Prompt(ctx, prompt); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	if observed != "2" {
		t.Fatalf("last observed message count = %q, want 2", observed)
	}
	state, err := agent.State()
	if err != nil || state.IsStreaming || len(state.Messages) != 2 {
		t.Fatalf("settled state/error = %#v/%v", state, err)
	}
}

func TestAgent_PromptAndWaitForIdleAwaitSlowSubscriber(t *testing.T) {
	agent := newSubscriptionTestAgent(t)
	listenerEntered := make(chan struct{})
	releaseListener := make(chan struct{})
	secondCalled := make(chan struct{})
	var enterOnce sync.Once
	var secondOnce sync.Once

	_, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentEnd {
			enterOnce.Do(func() { close(listenerEntered) })
			<-releaseListener
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(first) returned error: %v", err)
	}
	_, err = agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentEnd {
			secondOnce.Do(func() { close(secondCalled) })
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(second) returned error: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- agent.Prompt(context.Background())
	}()
	<-listenerEntered
	assertChannelOpen(t, promptDone, "Prompt returned before slow subscriber")
	assertChannelOpen(t, secondCalled, "second subscriber ran concurrently")
	state, err := agent.State()
	if err != nil || !state.IsStreaming {
		t.Fatalf("active state/error = %#v/%v", state, err)
	}

	idleDone := make(chan error, 1)
	go func() { idleDone <- agent.WaitForIdle(context.Background()) }()
	assertChannelOpen(t, idleDone, "WaitForIdle returned before subscriber")
	close(releaseListener)
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	if err := <-idleDone; err != nil {
		t.Fatalf("WaitForIdle() returned error: %v", err)
	}
	<-secondCalled
}

func assertChannelOpen[T any](t *testing.T, channel <-chan T, message string) {
	t.Helper()
	select {
	case <-channel:
		t.Fatal(message)
	default:
	}
}

func TestAgent_isolatesSubscriberErrorsAndPanics(t *testing.T) {
	agent := newSubscriptionTestAgent(t)
	subscriberError := errors.New("subscriber failed")
	panicError := errors.New("subscriber panicked")
	var survivingEvents []AgentEventKind

	unsubscribeError, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentStart {
			return subscriberError
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(error) returned error: %v", err)
	}
	unsubscribePanic, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventTurnStart {
			panic(panicError)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(panic) returned error: %v", err)
	}
	_, err = agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		survivingEvents = append(survivingEvents, event.Kind())
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe(survivor) returned error: %v", err)
	}

	err = agent.Prompt(context.Background())
	if !errors.Is(err, ErrAgentSubscriber) || !errors.Is(err, subscriberError) || !errors.Is(err, panicError) {
		t.Fatalf("Prompt() error = %v, want isolated subscriber errors", err)
	}
	var typed *AgentSubscriberError
	if !errors.As(err, &typed) {
		t.Fatalf("Prompt() error type = %T, want *AgentSubscriberError", err)
	}
	wantEvents := []AgentEventKind{
		AgentEventAgentStart, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventAgentEnd,
	}
	if !reflect.DeepEqual(survivingEvents, wantEvents) {
		t.Fatalf("surviving events = %#v, want %#v", survivingEvents, wantEvents)
	}

	unsubscribeError()
	unsubscribePanic()
	if err := agent.SetSystemPrompt("owner 仍可用"); err != nil {
		t.Fatalf("SetSystemPrompt() after subscriber failures returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("second Prompt() returned error: %v", err)
	}
}

func newSubscriptionTestAgent(t *testing.T) *Agent {
	t.Helper()
	agent, err := NewAgent(AgentOptions{
		LoopConfig: LoopConfig{
			Stream: staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}),
		},
	})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	return agent
}
