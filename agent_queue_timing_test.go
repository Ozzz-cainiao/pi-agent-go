package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestAgent_injectsSteeringOnlyAfterCompleteToolBatch(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	executed := make([]string, 0, 2)
	tools := []Tool{
		&queueTimingTool{name: "first", started: firstStarted, release: releaseFirst, executed: &executed},
		&queueTimingTool{name: "second", executed: &executed},
	}
	modelCalls := 0
	sawQueuedAfterBatch := false
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Tools: tools},
		LoopConfig: LoopConfig{
			ToolExecution: ToolExecutionModeSequential,
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				if modelCalls == 1 {
					return AssistantMessage{
						Content: []AssistantContent{
							ToolCall{ID: "call-1", Name: "first"},
							ToolCall{ID: "call-2", Name: "second"},
						},
						StopReason: StopReasonToolUse,
					}, nil
				}
				sawQueuedAfterBatch = reflect.DeepEqual(executed, []string{"first", "second"}) && containsUserText(modelContext.Messages, "steer")
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
		},
	})
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	<-firstStarted
	if err := agent.Steer(queueUserMessage("steer")); err != nil {
		t.Fatalf("Steer() returned error: %v", err)
	}
	close(releaseFirst)
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	if modelCalls != 2 || !sawQueuedAfterBatch {
		t.Fatalf("calls/saw queued after batch = %d/%v, want 2/true", modelCalls, sawQueuedAfterBatch)
	}
}

type queueTimingTool struct {
	name     string
	started  chan struct{}
	release  chan struct{}
	executed *[]string
}

func (tool *queueTimingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool *queueTimingTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	if tool.started != nil {
		close(tool.started)
		<-tool.release
	}
	*tool.executed = append(*tool.executed, tool.name)
	return ToolResult{}, nil
}
