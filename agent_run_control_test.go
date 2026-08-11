package agent

import (
	"context"
	"errors"
	"testing"
)

func TestAgent_busyOperationsDoNotModifyTranscript(t *testing.T) {
	streamContext := make(chan context.Context, 1)
	agent := newRunControlAgent(t, func(
		ctx context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		streamContext <- ctx
		<-ctx.Done()
		return AssistantMessage{}, ctx.Err()
	})
	first := UserMessage{Content: []UserContent{TextContent{Text: "first"}}}
	second := UserMessage{Content: []UserContent{TextContent{Text: "second"}}}
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background(), first) }()
	<-streamContext

	if err := agent.Prompt(context.Background(), second); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("concurrent Prompt() error = %v, want ErrAgentBusy", err)
	}
	if err := agent.Continue(context.Background()); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("concurrent Continue() error = %v, want ErrAgentBusy", err)
	}
	if err := agent.Reset(); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("concurrent Reset() error = %v, want ErrAgentBusy", err)
	}
	if err := agent.Abort(); err != nil {
		t.Fatalf("Abort() returned error: %v", err)
	}
	if err := <-promptDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("active Prompt() error = %v, want context cancellation", err)
	}

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.IsStreaming || len(state.Messages) != 1 || userTextAt(t, state.Messages, 0).Text != "first" {
		t.Fatalf("state after rejected operations = %#v", state)
	}
}

func TestAgent_AbortIsIdempotentAndStateRecovers(t *testing.T) {
	streamContext := make(chan context.Context, 1)
	modelCalls := 0
	agent := newRunControlAgent(t, func(
		ctx context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++
		if modelCalls == 1 {
			streamContext <- ctx
			<-ctx.Done()
			return AssistantMessage{}, ctx.Err()
		}
		return AssistantMessage{StopReason: StopReasonStop}, nil
	})
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	runContext := <-streamContext

	if err := agent.Abort(); err != nil {
		t.Fatalf("first Abort() returned error: %v", err)
	}
	if err := agent.Abort(); err != nil {
		t.Fatalf("second Abort() returned error: %v", err)
	}
	select {
	case <-runContext.Done():
	default:
		t.Fatal("Stream context was not canceled")
	}
	if err := <-promptDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Prompt() error = %v, want context cancellation", err)
	}
	if err := agent.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() returned error: %v", err)
	}
	if err := agent.Abort(); err != nil {
		t.Fatalf("idle Abort() returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("Prompt() after abort returned error: %v", err)
	}
	state, err := agent.State()
	if err != nil || state.IsStreaming || state.StreamingMessage != nil || len(state.PendingToolCalls) != 0 {
		t.Fatalf("recovered state/error = %#v/%v", state, err)
	}
}

func TestAgent_ResetClearsRuntimeStateAndKeepsConfiguration(t *testing.T) {
	tool := stateNamedTool{name: "search"}
	agent, err := NewAgent(AgentOptions{
		InitialState: &AgentInitialState{
			SystemPrompt: "system",
			Model:        "stable-model",
			Tools:        []Tool{tool},
			Messages: []AgentMessage{
				UserMessage{Content: []UserContent{TextContent{Text: "history"}}},
			},
		},
		LoopConfig: LoopConfig{
			Stream: staticAssistantStream(AssistantMessage{
				StopReason:   StopReasonError,
				ErrorMessage: "旧错误",
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	beforeReset, err := agent.State()
	if err != nil || beforeReset.ErrorMessage != "旧错误" {
		t.Fatalf("state before Reset = %#v/%v", beforeReset, err)
	}
	if err := agent.Reset(); err != nil {
		t.Fatalf("Reset() returned error: %v", err)
	}

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if len(state.Messages) != 0 || state.IsStreaming || state.StreamingMessage != nil || len(state.PendingToolCalls) != 0 || state.ErrorMessage != "" {
		t.Fatalf("reset runtime state = %#v", state)
	}
	if state.SystemPrompt != "system" || state.Model != "stable-model" || state.Tools[0].Definition().Name != "search" {
		t.Fatalf("reset configuration = %#v", state)
	}
}

func newRunControlAgent(t *testing.T, stream StreamFunc) *Agent {
	t.Helper()
	agent, err := NewAgent(AgentOptions{LoopConfig: LoopConfig{Stream: stream}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	return agent
}
