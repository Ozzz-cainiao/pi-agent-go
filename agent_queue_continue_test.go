package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestAgent_ContinueFromAssistantUsesQueuedSteering(t *testing.T) {
	modelCalls := 0
	firstCallSawSecond := false
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{
			Messages: []AgentMessage{AssistantMessage{StopReason: StopReasonStop}},
		},
		SteeringMode: QueueModeOneAtATime,
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelCalls++
			if modelCalls == 1 {
				firstCallSawSecond = containsUserText(modelContext.Messages, "second")
			}
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Steer(queueUserMessage("first")); err != nil {
		t.Fatalf("Steer(first) returned error: %v", err)
	}
	if err := agent.Steer(queueUserMessage("second")); err != nil {
		t.Fatalf("Steer(second) returned error: %v", err)
	}

	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if modelCalls != 2 || firstCallSawSecond {
		t.Fatalf("calls/first saw second = %d/%v, want 2/false", modelCalls, firstCallSawSecond)
	}
}

func TestAgent_ContinueFromToolResultUsesLowLevelContinuation(t *testing.T) {
	history := []AgentMessage{
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{ToolCallID: "call-1", ToolName: "search"},
	}
	sawHistory := false
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Messages: history},
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			sawHistory = len(modelContext.Messages) == len(history)
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if !sawHistory {
		t.Fatal("Continue() did not pass existing tool-result history")
	}
}

func TestAgent_stopAfterTurnPreservesFollowUpForContinue(t *testing.T) {
	modelCalls := 0
	agent := newQueueTestAgent(t, AgentOptions{
		LoopConfig: LoopConfig{
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			ShouldStopAfterTurn: func(context.Context, ShouldStopAfterTurnContext) (bool, error) {
				return true, nil
			},
		},
	})
	if err := agent.FollowUp(queueUserMessage("later")); err != nil {
		t.Fatalf("FollowUp() returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	queued, err := agent.HasQueuedMessages()
	if err != nil || !queued {
		t.Fatalf("HasQueuedMessages() = %v/%v, want true/nil", queued, err)
	}
	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("Model calls = %d, want 2", modelCalls)
	}
}

func TestAgent_busyContinueDoesNotDrainQueue(t *testing.T) {
	streamStarted := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	agent := newQueueTestAgent(t, AgentOptions{
		LoopConfig: LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			startedOnce.Do(func() { close(streamStarted) })
			<-release
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Steer(queueUserMessage("keep")); err != nil {
		t.Fatalf("Steer() returned error: %v", err)
	}
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	<-streamStarted
	if err := agent.Continue(context.Background()); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("Continue() error = %v, want ErrAgentBusy", err)
	}
	queued, err := agent.HasQueuedMessages()
	if err != nil || queued {
		t.Fatalf("active run should have drained initial steering before Model: %v/%v", queued, err)
	}
	if err := agent.Steer(queueUserMessage("not-lost")); err != nil {
		t.Fatalf("Steer() during run returned error: %v", err)
	}
	if err := agent.Continue(context.Background()); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("second Continue() error = %v, want ErrAgentBusy", err)
	}
	queued, err = agent.HasQueuedMessages()
	if err != nil || !queued {
		t.Fatalf("queued message after busy Continue = %v/%v, want true/nil", queued, err)
	}
	close(release)
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
}
