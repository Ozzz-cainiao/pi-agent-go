package agent

import (
	"context"
	"errors"
	"testing"
)

func TestRunAgentLoop_stopsImmediatelyWhenAssistantEventSinkFails(t *testing.T) {
	sinkError := errors.New("sink failed")
	emittedAfterFailure := false
	stream := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		emit AssistantMessageEventSink,
	) (AssistantMessage, error) {
		if err := emit(AssistantStartEvent{
			Partial: AssistantMessage{StopReason: StopReasonPending},
		}); err != nil {
			return AssistantMessage{}, err
		}

		emittedAfterFailure = true

		return AssistantMessage{StopReason: StopReasonStop}, nil
	})
	failingSinkCalls := 0
	sink := AgentEventSink(func(event AgentEvent) error {
		if event.Kind() != AgentEventMessageStart {
			return nil
		}
		failingSinkCalls++
		return sinkError
	})

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: stream},
		sink,
	)

	if !errors.Is(err, sinkError) {
		t.Fatalf("RunAgentLoop() error = %v, want sink error", err)
	}
	if !errors.Is(err, ErrAgentEventSink) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrAgentEventSink", err)
	}
	if emittedAfterFailure {
		t.Fatal("StreamFunc continued after AssistantMessageEventSink failure")
	}
	if failingSinkCalls != 1 {
		t.Fatalf("failing sink call count = %d, want 1", failingSinkCalls)
	}
}
