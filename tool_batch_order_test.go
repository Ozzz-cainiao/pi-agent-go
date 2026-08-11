package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

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
