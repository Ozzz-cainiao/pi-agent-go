package agent

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestAgent_serializesConcurrentStateWrites(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	const writers = 100
	var wait sync.WaitGroup
	wait.Add(writers)
	errorsByWriter := make(chan error, writers)
	for index := range writers {
		go func() {
			defer wait.Done()
			errorsByWriter <- agent.AppendMessage(CustomMessage{
				Kind:    "concurrent",
				Payload: map[string]string{"id": fmt.Sprint(index)},
			})
		}()
	}
	wait.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("AppendMessage() returned error: %v", err)
		}
	}

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if len(state.Messages) != writers {
		t.Fatalf("message count = %d, want %d", len(state.Messages), writers)
	}
}

func TestAgent_CloseIsIdempotentAndRejectsNewOperations(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	if err := agent.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}
	if err := agent.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}
	if _, err := agent.State(); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("State() error = %v, want ErrAgentClosed", err)
	}
}
