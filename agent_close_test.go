package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgent_CloseCancelsActiveRunAndSettlesWaiters(t *testing.T) {
	streamStarted := make(chan struct{})
	streamExited := make(chan struct{})
	var streamCalls atomic.Int32
	agent, err := NewAgent(AgentOptions{LoopConfig: LoopConfig{Stream: func(
		ctx context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		streamCalls.Add(1)
		close(streamStarted)
		<-ctx.Done()
		close(streamExited)
		return AssistantMessage{}, ctx.Err()
	}}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	awaitSignal(t, streamStarted, "stream start")

	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	closeDone := make(chan error, 1)
	go func() { closeDone <- agent.Close() }()

	if err := awaitCloseResult(t, closeDone, "Close"); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if err := agent.WaitForIdle(waitContext); err != nil {
		t.Fatalf("WaitForIdle() after Close returned error: %v", err)
	}
	awaitSignal(t, streamExited, "stream exit")
	if err := <-promptDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Prompt() error = %v, want context cancellation", err)
	}
	if calls := streamCalls.Load(); calls != 1 {
		t.Fatalf("stream calls = %d, want 1", calls)
	}
	if err := agent.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("Prompt() after Close error = %v, want ErrAgentClosed", err)
	}
	if _, err := agent.State(); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("State() after Close error = %v, want ErrAgentClosed", err)
	}
}

func TestAgent_CloseFromAgentEndSubscriberDoesNotDeadlock(t *testing.T) {
	var highLevelAgent *Agent
	subscriberEntered := make(chan struct{})
	subscriberCloseDone := make(chan error, 1)
	externalCloseDone := make(chan error, 1)
	promptDone := make(chan error, 1)

	var err error
	highLevelAgent, err = NewAgent(AgentOptions{LoopConfig: LoopConfig{
		Stream: staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}),
	}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	_, err = highLevelAgent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentEnd {
			close(subscriberEntered)
			subscriberCloseDone <- highLevelAgent.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}

	go func() { promptDone <- highLevelAgent.Prompt(context.Background()) }()
	go func() {
		<-subscriberEntered
		externalCloseDone <- highLevelAgent.Close()
	}()

	if err := awaitCloseResult(t, subscriberCloseDone, "subscriber Close"); err != nil {
		t.Fatalf("subscriber Close() returned error: %v", err)
	}
	if err := awaitCloseResult(t, externalCloseDone, "external Close"); err != nil {
		t.Fatalf("external Close() returned error: %v", err)
	}
	if err := awaitCloseResult(t, promptDone, "Prompt"); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	if err := highLevelAgent.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() returned error: %v", err)
	}
}

func awaitCloseResult(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s did not settle within one second", name)
		return nil
	}
}
