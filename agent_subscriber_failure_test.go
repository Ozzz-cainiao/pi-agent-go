package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

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
