package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAgent_streamErrorProjectsCompleteFailureLifecycle(t *testing.T) {
	streamError := errors.New("provider exploded")
	agent := newRunControlAgent(t, func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
		return AssistantMessage{}, streamError
	})
	var events []AgentEvent
	_, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}

	prompt := queueUserMessage("hello")
	err = agent.Prompt(context.Background(), prompt)
	if !errors.Is(err, streamError) {
		t.Fatalf("Prompt() error = %v, want stream sentinel", err)
	}
	wantKinds := []AgentEventKind{
		AgentEventAgentStart, AgentEventTurnStart,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventMessageStart, AgentEventMessageEnd,
		AgentEventTurnEnd, AgentEventAgentEnd,
	}
	if got := agentEventKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("failure event kinds = %#v, want %#v", got, wantKinds)
	}
	end, ok := events[len(events)-1].(AgentEndEvent)
	if !ok || len(end.Messages) != 1 {
		t.Fatalf("agent_end = %#v, want one failure message", events[len(events)-1])
	}
	failure := assistantMessageAt(t, end.Messages, 0)
	if failure.StopReason != StopReasonError || failure.ErrorMessage == "" {
		t.Fatalf("failure message = %#v", failure)
	}
	state, err := agent.State()
	if err != nil || state.IsStreaming || state.ErrorMessage == "" || len(state.Messages) != 2 {
		t.Fatalf("failure state/error = %#v/%v", state, err)
	}
	if tail := assistantMessageAt(t, state.Messages, 1); tail.StopReason != StopReasonError {
		t.Fatalf("history tail = %#v, want error assistant", tail)
	}
}

func TestAgent_cancelFailureSettlesAfterAgentEndSubscriber(t *testing.T) {
	streamStarted := make(chan struct{})
	agent := newRunControlAgent(t, func(
		ctx context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		close(streamStarted)
		<-ctx.Done()
		return AssistantMessage{}, ctx.Err()
	})
	agentEndEntered := make(chan struct{})
	releaseSubscriber := make(chan struct{})
	_, err := agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentEnd {
			close(agentEndEntered)
			<-releaseSubscriber
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	<-streamStarted
	if err := agent.Abort(); err != nil {
		t.Fatalf("Abort() returned error: %v", err)
	}
	<-agentEndEntered

	idleDone := make(chan error, 1)
	go func() { idleDone <- agent.WaitForIdle(context.Background()) }()
	assertChannelOpen(t, promptDone, "Prompt settled before agent_end subscriber")
	assertChannelOpen(t, idleDone, "WaitForIdle settled before agent_end subscriber")
	close(releaseSubscriber)
	if err := <-promptDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Prompt() error = %v, want context cancellation", err)
	}
	if err := <-idleDone; err != nil {
		t.Fatalf("WaitForIdle() returned error: %v", err)
	}
	state, err := agent.State()
	if err != nil || state.IsStreaming {
		t.Fatalf("settled state/error = %#v/%v", state, err)
	}
	if tail := assistantMessageAt(t, state.Messages, len(state.Messages)-1); tail.StopReason != StopReasonAborted {
		t.Fatalf("cancel tail = %#v, want aborted", tail)
	}
}

func TestAgent_toolErrorStaysModelVisibleAndSettles(t *testing.T) {
	toolError := errors.New("search unavailable")
	tool := &failureTool{name: "search", err: toolError}
	modelCalls := 0
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Tools: []Tool{tool}},
		LoopConfig: LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			modelCalls++
			if modelCalls == 1 {
				return AssistantMessage{
					Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "search"}},
					StopReason: StopReasonToolUse,
				}, nil
			}
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("Prompt() returned error for model-visible tool failure: %v", err)
	}
	state, err := agent.State()
	if err != nil || state.IsStreaming || modelCalls != 2 {
		t.Fatalf("tool failure state/calls/error = %#v/%d/%v", state, modelCalls, err)
	}
	result, ok := state.Messages[1].(ToolResultMessage)
	if !ok || !result.IsError {
		t.Fatalf("tool result = %#v, want error ToolResultMessage", state.Messages[1])
	}
}

type failureTool struct {
	name string
	err  error
}

func (tool *failureTool) Definition() ToolDefinition { return ToolDefinition{Name: tool.name} }

func (tool *failureTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	return ToolResult{}, tool.err
}

func assistantMessageAt(t *testing.T, messages []AgentMessage, index int) AssistantMessage {
	t.Helper()
	message, ok := messages[index].(AssistantMessage)
	if !ok {
		t.Fatalf("message[%d] type = %T, want AssistantMessage", index, messages[index])
	}
	return message
}
