package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunAgentLoop_parallelCancellationSettlesEveryWorkerAtSourceIndex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	neverRelease := make(chan struct{})
	finished := make(chan struct {
		messages []AgentMessage
		err      error
	}, 1)

	go func() {
		messages, err := RunAgentLoop(
			ctx, nil,
			AgentContext{Tools: []Tool{
				controlledBatchTool{name: "first", started: firstStarted, release: neverRelease},
				controlledBatchTool{name: "second", started: secondStarted, release: neverRelease},
			}},
			LoopConfig{Stream: twoToolBatchStream()},
			nil,
		)
		finished <- struct {
			messages []AgentMessage
			err      error
		}{messages: messages, err: err}
	}()

	awaitSignal(t, firstStarted, "first cancellable tool start")
	awaitSignal(t, secondStarted, "second cancellable tool start")
	cancel()

	select {
	case result := <-finished:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("RunAgentLoop() error = %v, want context.Canceled", result.err)
		}
		if len(result.messages) < 3 {
			t.Fatalf("messages = %#v, want assistant and indexed tool errors", result.messages)
		}
		for index, id := range []string{"call-1", "call-2"} {
			message, ok := result.messages[index+1].(ToolResultMessage)
			if !ok {
				t.Fatalf("message[%d] type = %T, want ToolResultMessage", index+1, result.messages[index+1])
			}
			if message.ToolCallID != id || !message.IsError || !strings.Contains(toolResultText(t, message), context.Canceled.Error()) {
				t.Fatalf("message[%d] = %#v, want canceled result for %s", index+1, message, id)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parallel cancellation did not settle workers")
	}
}

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

var errParallelTool = errors.New("parallel tool failed")

func TestRunAgentLoop_parallelEndUsesCompletionOrderButMessagesUseSourceOrder(t *testing.T) {
	for iteration := range 100 {
		firstStarted := make(chan struct{})
		secondStarted := make(chan struct{})
		firstRelease := make(chan struct{})
		secondRelease := make(chan struct{})
		endOrder := make(chan string, 2)
		type loopResult struct {
			messages []AgentMessage
			err      error
		}
		finished := make(chan loopResult, 1)

		go func() {
			messages, err := RunAgentLoop(
				context.Background(), nil,
				AgentContext{Tools: []Tool{
					controlledBatchTool{name: "first", started: firstStarted, release: firstRelease, err: errParallelTool},
					controlledBatchTool{name: "second", started: secondStarted, release: secondRelease},
				}},
				LoopConfig{Stream: twoToolBatchStream()},
				func(event AgentEvent) error {
					if end, ok := event.(AgentToolExecutionEndEvent); ok {
						endOrder <- end.ToolCallID
					}
					return nil
				},
			)
			finished <- loopResult{messages: messages, err: err}
		}()

		awaitSignal(t, firstStarted, "first tool start")
		awaitSignal(t, secondStarted, "second tool start")
		close(secondRelease)
		if got := <-endOrder; got != "call-2" {
			t.Fatalf("iteration %d first end = %q, want call-2", iteration, got)
		}
		close(firstRelease)
		result := <-finished
		if result.err != nil {
			t.Fatalf("iteration %d RunAgentLoop() error: %v", iteration, result.err)
		}
		if got := <-endOrder; got != "call-1" {
			t.Fatalf("iteration %d second end = %q, want call-1", iteration, got)
		}
		firstResult := toolResultMessageAt(t, result.messages, 1)
		secondResult := toolResultMessageAt(t, result.messages, 2)
		gotIDs := []string{firstResult.ToolCallID, secondResult.ToolCallID}
		if !reflect.DeepEqual(gotIDs, []string{"call-1", "call-2"}) {
			t.Fatalf("iteration %d result order = %#v, want source order", iteration, gotIDs)
		}
		if !firstResult.IsError || !strings.Contains(toolResultText(t, firstResult), errParallelTool.Error()) {
			t.Fatalf("iteration %d first result = %#v, want source-indexed error", iteration, firstResult)
		}
		if secondResult.IsError {
			t.Fatalf("iteration %d second result = %#v, want success", iteration, secondResult)
		}
	}
}

func toolResultMessageAt(
	t *testing.T,
	messages []AgentMessage,
	index int,
) ToolResultMessage {
	t.Helper()
	message, ok := messages[index].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[%d] type = %T, want ToolResultMessage", index, messages[index])
	}
	return message
}

func TestAgent_parallelFailureProjectsFailureAndClearsPendingTools(t *testing.T) {
	tools := []Tool{
		&invalidUpdateTool{name: "invalid"},
		&failureTool{name: "normal"},
	}
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Tools: tools},
		LoopConfig: LoopConfig{Stream: staticAssistantStream(AssistantMessage{
			Content: []AssistantContent{
				ToolCall{ID: "call-1", Name: "invalid"},
				ToolCall{ID: "call-2", Name: "normal"},
			},
			StopReason: StopReasonToolUse,
		})},
	})

	err := agent.Prompt(context.Background())
	if !errors.Is(err, ErrAgentEventSink) {
		t.Fatalf("Prompt() error = %v, want ErrAgentEventSink", err)
	}
	state, stateError := agent.State()
	if stateError != nil || state.IsStreaming || len(state.PendingToolCalls) != 0 {
		t.Fatalf("parallel failure state/error = %#v/%v", state, stateError)
	}
	if tail := assistantMessageAt(t, state.Messages, len(state.Messages)-1); tail.StopReason != StopReasonError {
		t.Fatalf("parallel failure tail = %#v, want error", tail)
	}
}

type invalidUpdateDetails struct {
	values map[string]string
}

type invalidUpdateTool struct{ name string }

func (tool *invalidUpdateTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool *invalidUpdateTool) Execute(
	_ context.Context,
	_ ToolCall,
	onUpdate ToolUpdateFunc,
) (ToolResult, error) {
	onUpdate(ToolResult{Details: invalidUpdateDetails{values: map[string]string{"state": "running"}}})
	return ToolResult{}, nil
}
