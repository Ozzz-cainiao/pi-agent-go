package agent

import (
	"context"
	"errors"
	"sync"
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

	waitObserved := make(chan struct{})
	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	observedContext := &doneObservedContext{Context: waitContext, observed: waitObserved}
	closeDone := make(chan error, 1)
	go func() {
		<-waitObserved
		closeDone <- agent.Close()
	}()

	if err := agent.WaitForIdle(observedContext); err != nil {
		t.Fatalf("WaitForIdle() during Close returned error: %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() returned error: %v", err)
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
	if err := agent.WaitForIdle(context.Background()); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("WaitForIdle() after Close error = %v, want ErrAgentClosed", err)
	}
}

type doneObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (ctx *doneObservedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}
