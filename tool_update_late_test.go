package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

type lateUpdateTool struct {
	callback chan<- ToolUpdateFunc
}

func (lateUpdateTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "first"}
}

func (tool lateUpdateTool) Execute(
	_ context.Context,
	_ ToolCall,
	onUpdate ToolUpdateFunc,
) (ToolResult, error) {
	onUpdate(ToolResult{Content: []ToolResultContent{TextContent{Text: "during"}}})
	tool.callback <- onUpdate
	return ToolResult{Content: []ToolResultContent{TextContent{Text: "first done"}}}, nil
}

type barrierTool struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (barrierTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "second"}
}

func (tool barrierTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	close(tool.started)
	<-tool.release
	return ToolResult{Content: []ToolResultContent{TextContent{Text: "second done"}}}, nil
}

func TestRunAgentLoop_discardsLateToolUpdateWhileAnotherToolRuns(t *testing.T) {
	callback := make(chan ToolUpdateFunc, 1)
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	var updateMutex sync.Mutex
	var updates []string
	finished := make(chan error, 1)
	modelCall := 0

	go func() {
		_, err := RunAgentLoop(
			context.Background(),
			nil,
			AgentContext{Tools: []Tool{
				lateUpdateTool{callback: callback},
				barrierTool{started: secondStarted, release: releaseSecond},
			}},
			LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
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
			}},
			func(event AgentEvent) error {
				update, ok := event.(AgentToolExecutionUpdateEvent)
				if !ok {
					return nil
				}
				updateMutex.Lock()
				updates = append(updates, toolResultContentText(t, update.PartialResult))
				updateMutex.Unlock()
				return nil
			},
		)
		finished <- err
	}()

	var lateCallback ToolUpdateFunc
	select {
	case lateCallback = <-callback:
	case <-time.After(2 * time.Second):
		t.Fatal("first tool did not expose update callback")
	}
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second tool did not start")
	}
	lateCallback(ToolResult{Content: []ToolResultContent{TextContent{Text: "late"}}})
	close(releaseSecond)
	if err := <-finished; err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}

	updateMutex.Lock()
	defer updateMutex.Unlock()
	if len(updates) != 1 || updates[0] != "during" {
		t.Fatalf("updates = %#v, want only settle-before update", updates)
	}
}

func toolResultContentText(t *testing.T, result ToolResult) string {
	t.Helper()
	text, ok := result.Content[0].(TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T, want TextContent", result.Content[0])
	}
	return text.Text
}
