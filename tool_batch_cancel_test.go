package agent

import (
	"context"
	"errors"
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
