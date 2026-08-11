package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

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
