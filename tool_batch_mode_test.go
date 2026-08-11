package agent

import (
	"context"
	"testing"
	"time"
)

type controlledBatchTool struct {
	name    string
	mode    ToolExecutionMode
	started chan<- struct{}
	release <-chan struct{}
	err     error
}

func (tool controlledBatchTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name, ExecutionMode: tool.mode}
}

func (tool controlledBatchTool) Execute(
	ctx context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	close(tool.started)
	select {
	case <-tool.release:
		return ToolResult{Content: []ToolResultContent{TextContent{Text: tool.name}}}, tool.err
	case <-ctx.Done():
		return ToolResult{}, ctx.Err()
	}
}

func TestRunAgentLoop_executesBatchInParallelOnlyWhenEveryToolAllowsIt(t *testing.T) {
	tests := []struct {
		name       string
		configMode ToolExecutionMode
		firstMode  ToolExecutionMode
		secondMode ToolExecutionMode
		parallel   bool
	}{
		{name: "default parallel", parallel: true},
		{name: "explicit parallel", configMode: ToolExecutionModeParallel, parallel: true},
		{name: "config sequential", configMode: ToolExecutionModeSequential},
		{
			name: "tool sequential override", configMode: ToolExecutionModeParallel,
			firstMode: ToolExecutionModeSequential,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstStarted := make(chan struct{})
			secondStarted := make(chan struct{})
			firstRelease := make(chan struct{})
			secondRelease := make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				_, err := RunAgentLoop(
					context.Background(), nil,
					AgentContext{Tools: []Tool{
						controlledBatchTool{name: "first", mode: test.firstMode, started: firstStarted, release: firstRelease},
						controlledBatchTool{name: "second", mode: test.secondMode, started: secondStarted, release: secondRelease},
					}},
					LoopConfig{ToolExecution: test.configMode, Stream: twoToolBatchStream()},
					nil,
				)
				finished <- err
			}()

			awaitSignal(t, firstStarted, "first tool start")
			if test.parallel {
				awaitSignal(t, secondStarted, "parallel second tool start")
				close(firstRelease)
				close(secondRelease)
			} else {
				select {
				case <-secondStarted:
					t.Fatal("second tool started before sequential first tool settled")
				default:
				}
				close(firstRelease)
				awaitSignal(t, secondStarted, "sequential second tool start")
				close(secondRelease)
			}
			if err := awaitError(t, finished, "Agent Loop completion"); err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
		})
	}
}

func twoToolBatchStream() StreamFunc {
	modelCall := 0
	return func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
		modelCall++
		if modelCall == 1 {
			return AssistantMessage{
				Content: []AssistantContent{
					ToolCall{ID: "call-1", Name: "first"},
					ToolCall{ID: "call-2", Name: "second"},
				},
				StopReason: StopReasonToolUse,
			}, nil
		}
		return AssistantMessage{StopReason: StopReasonStop}, nil
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func awaitError(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}
