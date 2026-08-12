package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAgent_usesDefaultState(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "" || state.Model != UnknownModelID || state.ThinkingLevel != ThinkingLevelOff {
		t.Fatalf("default identity state = %#v", state)
	}
	if len(state.Tools) != 0 || len(state.Messages) != 0 || len(state.PendingToolCalls) != 0 {
		t.Fatalf("default collections = %#v", state)
	}
	if state.IsStreaming || state.StreamingMessage != nil || state.ErrorMessage != "" {
		t.Fatalf("default runtime state = %#v", state)
	}
}

func TestNewAgent_copiesCustomInitialState(t *testing.T) {
	arguments := map[string]any{"filters": map[string]any{"scope": "docs"}}
	messages := []AgentMessage{AssistantMessage{
		Content: []AssistantContent{ToolCall{ID: "call-1", Name: "search", Arguments: arguments}},
	}}
	tools := []Tool{stateNamedTool{name: "search"}}
	initial := AgentInitialState{
		SystemPrompt:  "系统提示词",
		Model:         ModelID("demo-model"),
		ThinkingLevel: ThinkingLevelHigh,
		Tools:         tools,
		Messages:      messages,
	}

	agent, err := NewAgent(AgentOptions{InitialState: &initial})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	filters, ok := arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", arguments["filters"])
	}
	filters["scope"] = "changed"
	tools[0] = stateNamedTool{name: "changed"}
	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "系统提示词" || state.Model != "demo-model" || state.ThinkingLevel != ThinkingLevelHigh {
		t.Fatalf("custom identity state = %#v", state)
	}
	if state.Tools[0].Definition().Name != "search" || len(state.PendingToolCalls) != 0 {
		t.Fatalf("custom collections = %#v", state)
	}
	if got := toolArgumentScope(t, state.Messages); got != "docs" {
		t.Fatalf("copied argument scope = %q, want docs", got)
	}
	if state.IsStreaming || state.StreamingMessage != nil || state.ErrorMessage != "" {
		t.Fatalf("custom runtime state = %#v", state)
	}
}

func TestAgent_stateMutators(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	tools := []Tool{stateNamedTool{name: "search"}}
	messages := []AgentMessage{UserMessage{Content: []UserContent{TextContent{Text: "你好"}}}}
	operations := []func() error{
		func() error { return agent.SetSystemPrompt("新提示词") },
		func() error { return agent.SetModel("stable-model") },
		func() error { return agent.SetThinkingLevel(ThinkingLevelMedium) },
		func() error { return agent.SetTools(tools) },
		func() error { return agent.ReplaceMessages(messages) },
		func() error { return agent.AppendMessage(AssistantMessage{StopReason: StopReasonStop}) },
	}
	for index, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("operation[%d] returned error: %v", index, err)
		}
	}
	tools[0] = stateNamedTool{name: "changed"}
	messages[0] = CustomMessage{Kind: "changed"}

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "新提示词" || state.Model != "stable-model" || state.ThinkingLevel != ThinkingLevelMedium {
		t.Fatalf("mutated identity state = %#v", state)
	}
	if state.Tools[0].Definition().Name != "search" || len(state.Messages) != 2 {
		t.Fatalf("mutated collections = %#v", state)
	}
	if err := agent.ClearMessages(); err != nil {
		t.Fatalf("ClearMessages() returned error: %v", err)
	}
	cleared, err := agent.State()
	if err != nil || len(cleared.Messages) != 0 {
		t.Fatalf("cleared state/error = %#v/%v", cleared, err)
	}
}

func TestAgent_StateReturnsDefensiveSnapshot(t *testing.T) {
	message := AssistantMessage{Content: []AssistantContent{
		ToolCall{ID: "call-1", Name: "search", Arguments: map[string]any{"filters": map[string]any{"scope": "docs"}}},
	}}
	agent, err := NewAgent(AgentOptions{InitialState: &AgentInitialState{
		Tools:    []Tool{stateNamedTool{name: "search"}},
		Messages: []AgentMessage{message},
	}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	first, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	first.Tools[0] = stateNamedTool{name: "changed"}
	first.PendingToolCalls["external"] = struct{}{}
	mutateToolArgumentScope(t, first.Messages, "changed")

	second, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if second.Tools[0].Definition().Name != "search" || len(second.PendingToolCalls) != 0 {
		t.Fatalf("snapshot mutation leaked into collections: %#v", second)
	}
	if got := toolArgumentScope(t, second.Messages); got != "docs" {
		t.Fatalf("snapshot mutation leaked into arguments: %q", got)
	}
}

type stateNamedTool struct{ name string }

func (tool stateNamedTool) Definition() ToolDefinition { return ToolDefinition{Name: tool.name} }

func (stateNamedTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	return ToolResult{}, nil
}

func closeAgent(t *testing.T, agent *Agent) {
	t.Helper()
	if err := agent.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func toolArgumentScope(t *testing.T, messages []AgentMessage) string {
	t.Helper()
	call := stateToolCall(t, messages)
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	scope, ok := filters["scope"].(string)
	if !ok {
		t.Fatalf("scope type = %T, want string", filters["scope"])
	}
	return scope
}

func mutateToolArgumentScope(t *testing.T, messages []AgentMessage, scope string) {
	t.Helper()
	call := stateToolCall(t, messages)
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	filters["scope"] = scope
}

func stateToolCall(t *testing.T, messages []AgentMessage) ToolCall {
	t.Helper()
	assistant, ok := messages[0].(AssistantMessage)
	if !ok {
		t.Fatalf("message type = %T, want AssistantMessage", messages[0])
	}
	call, ok := assistant.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("content type = %T, want ToolCall", assistant.Content[0])
	}
	return call
}

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
	if state.IsStreaming || len(state.Messages) != 2 || userTextAt(t, state.Messages, 0).Text != "first" {
		t.Fatalf("state after rejected operations = %#v", state)
	}
	if tail := assistantMessageAt(t, state.Messages, 1); tail.StopReason != StopReasonAborted {
		t.Fatalf("state tail = %#v, want aborted failure", tail)
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

func TestAgent_CloseCancelsActiveRunAndSettlesWaiters(t *testing.T) {
	streamStarted := make(chan struct{})
	streamExited := make(chan struct{})
	var streamCalls atomic.Int32
	agent, err := NewAgent(AgentOptions{LoopConfig: LoopConfig{Stream: func(
		ctx context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		streamCalls.Add(1)
		close(streamStarted)
		<-ctx.Done()
		close(streamExited)
		return AssistantMessage{}, ctx.Err()
	}}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	awaitSignal(t, streamStarted, "stream start")

	waitContext, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	closeDone := make(chan error, 1)
	go func() { closeDone <- agent.Close() }()

	if err := awaitCloseResult(t, closeDone, "Close"); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if err := agent.WaitForIdle(waitContext); err != nil {
		t.Fatalf("WaitForIdle() after Close returned error: %v", err)
	}
	awaitSignal(t, streamExited, "stream exit")
	if err := <-promptDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Prompt() error = %v, want context cancellation", err)
	}
	if calls := streamCalls.Load(); calls != 1 {
		t.Fatalf("stream calls = %d, want 1", calls)
	}
	if err := agent.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("Prompt() after Close error = %v, want ErrAgentClosed", err)
	}
	if _, err := agent.State(); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("State() after Close error = %v, want ErrAgentClosed", err)
	}
}

func TestAgent_CloseFromAgentEndSubscriberDoesNotDeadlock(t *testing.T) {
	var highLevelAgent *Agent
	subscriberEntered := make(chan struct{})
	subscriberCloseDone := make(chan error, 1)
	externalCloseDone := make(chan error, 1)
	promptDone := make(chan error, 1)

	var err error
	highLevelAgent, err = NewAgent(AgentOptions{LoopConfig: LoopConfig{
		Stream: staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}),
	}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	_, err = highLevelAgent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if event.Kind() == AgentEventAgentEnd {
			close(subscriberEntered)
			subscriberCloseDone <- highLevelAgent.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}

	go func() { promptDone <- highLevelAgent.Prompt(context.Background()) }()
	go func() {
		<-subscriberEntered
		externalCloseDone <- highLevelAgent.Close()
	}()

	if err := awaitCloseResult(t, subscriberCloseDone, "subscriber Close"); err != nil {
		t.Fatalf("subscriber Close() returned error: %v", err)
	}
	if err := awaitCloseResult(t, externalCloseDone, "external Close"); err != nil {
		t.Fatalf("external Close() returned error: %v", err)
	}
	if err := awaitCloseResult(t, promptDone, "Prompt"); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	if err := highLevelAgent.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() returned error: %v", err)
	}
}

func awaitCloseResult(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s did not settle within one second", name)
		return nil
	}
}

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
