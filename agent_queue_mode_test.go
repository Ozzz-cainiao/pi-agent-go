package agent

import (
	"context"
	"testing"
)

func TestAgent_steeringQueueModes(t *testing.T) {
	tests := []struct {
		name           string
		mode           QueueMode
		wantModelCalls int
		wantSecondSeen bool
	}{
		{name: "all", mode: QueueModeAll, wantModelCalls: 1, wantSecondSeen: true},
		{name: "one-at-a-time", mode: QueueModeOneAtATime, wantModelCalls: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			firstCallSawSecond := false
			agent := newQueueTestAgent(t, AgentOptions{
				SteeringMode: test.mode,
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

			if err := agent.Prompt(context.Background()); err != nil {
				t.Fatalf("Prompt() returned error: %v", err)
			}
			if modelCalls != test.wantModelCalls || firstCallSawSecond != test.wantSecondSeen {
				t.Fatalf("calls/second = %d/%v, want %d/%v", modelCalls, firstCallSawSecond, test.wantModelCalls, test.wantSecondSeen)
			}
			queued, err := agent.HasQueuedMessages()
			if err != nil || queued {
				t.Fatalf("HasQueuedMessages() = %v/%v, want false/nil", queued, err)
			}
		})
	}
}

func TestAgent_followUpQueueModes(t *testing.T) {
	tests := []struct {
		name           string
		mode           QueueMode
		wantModelCalls int
	}{
		{name: "all", mode: QueueModeAll, wantModelCalls: 2},
		{name: "one-at-a-time", mode: QueueModeOneAtATime, wantModelCalls: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			agent := newQueueTestAgent(t, AgentOptions{
				FollowUpMode: test.mode,
				LoopConfig: LoopConfig{Stream: func(
					_ context.Context,
					modelContext AgentContext,
					_ AssistantMessageEventSink,
				) (AssistantMessage, error) {
					modelCalls++
					if modelCalls == 1 && (containsUserText(modelContext.Messages, "first") || containsUserText(modelContext.Messages, "second")) {
						t.Fatal("follow-up was injected before Agent would stop")
					}
					return AssistantMessage{StopReason: StopReasonStop}, nil
				}},
			})
			if err := agent.FollowUp(queueUserMessage("first")); err != nil {
				t.Fatalf("FollowUp(first) returned error: %v", err)
			}
			if err := agent.FollowUp(queueUserMessage("second")); err != nil {
				t.Fatalf("FollowUp(second) returned error: %v", err)
			}

			if err := agent.Prompt(context.Background()); err != nil {
				t.Fatalf("Prompt() returned error: %v", err)
			}
			if modelCalls != test.wantModelCalls {
				t.Fatalf("Model calls = %d, want %d", modelCalls, test.wantModelCalls)
			}
		})
	}
}

func newQueueTestAgent(t *testing.T, options AgentOptions) *Agent {
	t.Helper()
	agent, err := NewAgent(options)
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	return agent
}

func queueUserMessage(text string) UserMessage {
	return UserMessage{Content: []UserContent{TextContent{Text: text}}}
}
