package agent

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestAgent_steeringQueueModes(t *testing.T) {
	tests := []struct {
		name           string
		mode           QueueMode
		wantModelCalls int
		wantSecondSeen bool
	}{
		{name: "all", mode: QueueModeAll, wantModelCalls: 1, wantSecondSeen: true},
		{name: "one-at-a-time", mode: QueueModeOneAtATime, wantModelCalls: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			firstCallSawSecond := false
			agent := newQueueTestAgent(t, AgentOptions{
				SteeringMode: test.mode,
				LoopConfig: LoopConfig{Stream: func(
					_ context.Context,
					modelContext AgentContext,
					_ AssistantMessageEventSink,
				) (AssistantMessage, error) {
					modelCalls++
					if modelCalls == 1 {
						firstCallSawSecond = containsUserText(modelContext.Messages, "second")
					}
					return AssistantMessage{StopReason: StopReasonStop}, nil
				}},
			})
			if err := agent.Steer(queueUserMessage("first")); err != nil {
				t.Fatalf("Steer(first) returned error: %v", err)
			}
			if err := agent.Steer(queueUserMessage("second")); err != nil {
				t.Fatalf("Steer(second) returned error: %v", err)
			}

			if err := agent.Prompt(context.Background()); err != nil {
				t.Fatalf("Prompt() returned error: %v", err)
			}
			if modelCalls != test.wantModelCalls || firstCallSawSecond != test.wantSecondSeen {
				t.Fatalf("calls/second = %d/%v, want %d/%v", modelCalls, firstCallSawSecond, test.wantModelCalls, test.wantSecondSeen)
			}
			queued, err := agent.HasQueuedMessages()
			if err != nil || queued {
				t.Fatalf("HasQueuedMessages() = %v/%v, want false/nil", queued, err)
			}
		})
	}
}

func TestAgent_followUpQueueModes(t *testing.T) {
	tests := []struct {
		name           string
		mode           QueueMode
		wantModelCalls int
	}{
		{name: "all", mode: QueueModeAll, wantModelCalls: 2},
		{name: "one-at-a-time", mode: QueueModeOneAtATime, wantModelCalls: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			agent := newQueueTestAgent(t, AgentOptions{
				FollowUpMode: test.mode,
				LoopConfig: LoopConfig{Stream: func(
					_ context.Context,
					modelContext AgentContext,
					_ AssistantMessageEventSink,
				) (AssistantMessage, error) {
					modelCalls++
					if modelCalls == 1 && (containsUserText(modelContext.Messages, "first") || containsUserText(modelContext.Messages, "second")) {
						t.Fatal("follow-up was injected before Agent would stop")
					}
					return AssistantMessage{StopReason: StopReasonStop}, nil
				}},
			})
			if err := agent.FollowUp(queueUserMessage("first")); err != nil {
				t.Fatalf("FollowUp(first) returned error: %v", err)
			}
			if err := agent.FollowUp(queueUserMessage("second")); err != nil {
				t.Fatalf("FollowUp(second) returned error: %v", err)
			}

			if err := agent.Prompt(context.Background()); err != nil {
				t.Fatalf("Prompt() returned error: %v", err)
			}
			if modelCalls != test.wantModelCalls {
				t.Fatalf("Model calls = %d, want %d", modelCalls, test.wantModelCalls)
			}
		})
	}
}

func newQueueTestAgent(t *testing.T, options AgentOptions) *Agent {
	t.Helper()
	agent, err := NewAgent(options)
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	return agent
}

func queueUserMessage(text string) UserMessage {
	return UserMessage{Content: []UserContent{TextContent{Text: text}}}
}

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

func TestAgent_ContinueFromAssistantUsesQueuedSteering(t *testing.T) {
	modelCalls := 0
	firstCallSawSecond := false
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{
			Messages: []AgentMessage{AssistantMessage{StopReason: StopReasonStop}},
		},
		SteeringMode: QueueModeOneAtATime,
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelCalls++
			if modelCalls == 1 {
				firstCallSawSecond = containsUserText(modelContext.Messages, "second")
			}
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Steer(queueUserMessage("first")); err != nil {
		t.Fatalf("Steer(first) returned error: %v", err)
	}
	if err := agent.Steer(queueUserMessage("second")); err != nil {
		t.Fatalf("Steer(second) returned error: %v", err)
	}

	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if modelCalls != 2 || firstCallSawSecond {
		t.Fatalf("calls/first saw second = %d/%v, want 2/false", modelCalls, firstCallSawSecond)
	}
}

func TestAgent_ContinueFromToolResultUsesLowLevelContinuation(t *testing.T) {
	history := []AgentMessage{
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{ToolCallID: "call-1", ToolName: "search"},
	}
	sawHistory := false
	agent := newQueueTestAgent(t, AgentOptions{
		InitialState: &AgentInitialState{Messages: history},
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			sawHistory = len(modelContext.Messages) == len(history)
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if !sawHistory {
		t.Fatal("Continue() did not pass existing tool-result history")
	}
}

func TestAgent_stopAfterTurnPreservesFollowUpForContinue(t *testing.T) {
	modelCalls := 0
	agent := newQueueTestAgent(t, AgentOptions{
		LoopConfig: LoopConfig{
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			ShouldStopAfterTurn: func(context.Context, ShouldStopAfterTurnContext) (bool, error) {
				return true, nil
			},
		},
	})
	if err := agent.FollowUp(queueUserMessage("later")); err != nil {
		t.Fatalf("FollowUp() returned error: %v", err)
	}
	if err := agent.Prompt(context.Background()); err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
	queued, err := agent.HasQueuedMessages()
	if err != nil || !queued {
		t.Fatalf("HasQueuedMessages() = %v/%v, want true/nil", queued, err)
	}
	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("Model calls = %d, want 2", modelCalls)
	}
}

func TestAgent_busyContinueDoesNotDrainQueue(t *testing.T) {
	streamStarted := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	agent := newQueueTestAgent(t, AgentOptions{
		LoopConfig: LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			startedOnce.Do(func() { close(streamStarted) })
			<-release
			return AssistantMessage{StopReason: StopReasonStop}, nil
		}},
	})
	if err := agent.Steer(queueUserMessage("keep")); err != nil {
		t.Fatalf("Steer() returned error: %v", err)
	}
	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.Prompt(context.Background()) }()
	<-streamStarted
	if err := agent.Continue(context.Background()); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("Continue() error = %v, want ErrAgentBusy", err)
	}
	queued, err := agent.HasQueuedMessages()
	if err != nil || queued {
		t.Fatalf("active run should have drained initial steering before Model: %v/%v", queued, err)
	}
	if err := agent.Steer(queueUserMessage("not-lost")); err != nil {
		t.Fatalf("Steer() during run returned error: %v", err)
	}
	if err := agent.Continue(context.Background()); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("second Continue() error = %v, want ErrAgentBusy", err)
	}
	queued, err = agent.HasQueuedMessages()
	if err != nil || !queued {
		t.Fatalf("queued message after busy Continue = %v/%v, want true/nil", queued, err)
	}
	close(release)
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() returned error: %v", err)
	}
}

func TestRunAgentLoop_injectsSteeringAfterCompleteToolBatch(t *testing.T) {
	queued := UserMessage{Content: []UserContent{TextContent{Text: "中途指令"}}}
	executed := make([]string, 0, 2)
	tools := []Tool{
		&recordingNamedTool{name: "first", executed: &executed},
		&recordingNamedTool{name: "second", executed: &executed},
	}
	modelCalls := 0
	polls := 0
	sawQueued := false
	var messageStarts []string

	messages, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: tools},
		LoopConfig{
			ToolExecution: ToolExecutionModeSequential,
			GetSteeringMessages: func(context.Context) ([]AgentMessage, error) {
				polls++
				if len(executed) == 2 && polls == 2 {
					return []AgentMessage{queued}, nil
				}
				return nil, nil
			},
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
				sawQueued = containsUserText(modelContext.Messages, "中途指令")
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
		},
		func(event AgentEvent) error {
			start, ok := event.(AgentMessageStartEvent)
			if !ok {
				return nil
			}
			switch message := start.Message.(type) {
			case ToolResultMessage:
				messageStarts = append(messageStarts, "tool:"+message.ToolCallID)
			case UserMessage:
				if containsUserText([]AgentMessage{message}, "中途指令") {
					messageStarts = append(messageStarts, "queued")
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(executed, []string{"first", "second"}) || !sawQueued {
		t.Fatalf("executed = %#v, saw queued = %v", executed, sawQueued)
	}
	if !reflect.DeepEqual(messageStarts, []string{"tool:call-1", "tool:call-2", "queued"}) {
		t.Fatalf("message starts = %#v, want tool results before queued", messageStarts)
	}
	if modelCalls != 2 || polls != 3 || len(messages) != 5 {
		t.Fatalf("calls/messages = model:%d polls:%d messages:%d, want 2/3/5", modelCalls, polls, len(messages))
	}
}

func TestRunAgentLoop_processesFollowUpOnlyWhenOtherwiseStopped(t *testing.T) {
	followUp := UserMessage{Content: []UserContent{TextContent{Text: "追问"}}}
	modelCalls := 0
	polls := 0
	secondTurnSawFollowUp := false

	messages, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				if modelCalls == 2 {
					secondTurnSawFollowUp = containsUserText(modelContext.Messages, "追问")
				}
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			GetFollowUpMessages: func(context.Context) ([]AgentMessage, error) {
				polls++
				if polls == 1 {
					return []AgentMessage{followUp}, nil
				}
				return nil, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 2 || polls != 2 || !secondTurnSawFollowUp || len(messages) != 3 {
		t.Fatalf("calls/state = model:%d polls:%d saw:%v messages:%d, want 2/2/true/3", modelCalls, polls, secondTurnSawFollowUp, len(messages))
	}
}

type recordingNamedTool struct {
	name     string
	executed *[]string
}

func (tool *recordingNamedTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool *recordingNamedTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	*tool.executed = append(*tool.executed, tool.name)
	return ToolResult{}, nil
}

func containsUserText(messages []AgentMessage, want string) bool {
	for _, candidate := range messages {
		message, ok := candidate.(UserMessage)
		if !ok {
			continue
		}
		for _, candidateContent := range message.Content {
			content, ok := candidateContent.(TextContent)
			if ok && content.Text == want {
				return true
			}
		}
	}
	return false
}

func TestAgent_ContinueUsesExistingHistoryWithoutReEmittingIt(t *testing.T) {
	history := []AgentMessage{
		UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{ToolCallID: "call-1", ToolName: "search"},
	}
	response := AssistantMessage{
		Content:    []AssistantContent{TextContent{Text: "回答"}},
		StopReason: StopReasonStop,
	}
	var modelMessages []AgentMessage
	agent, err := NewAgent(AgentOptions{
		InitialState: &AgentInitialState{Messages: history},
		LoopConfig: LoopConfig{Stream: func(
			_ context.Context,
			modelContext AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelMessages = modelContext.Messages
			return response, nil
		}},
	})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })
	var messageEnds []AgentMessage
	_, err = agent.Subscribe(func(_ context.Context, event AgentEvent) error {
		if end, ok := event.(AgentMessageEndEvent); ok {
			messageEnds = append(messageEnds, end.Message)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() returned error: %v", err)
	}
	before, err := agent.State()
	if err != nil {
		t.Fatalf("State() before Continue returned error: %v", err)
	}

	if err := agent.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() returned error: %v", err)
	}
	if !reflect.DeepEqual(modelMessages, before.Messages) || !reflect.DeepEqual(messageEnds, []AgentMessage{response}) {
		t.Fatalf("Model/events = %#v/%#v, want history/new response", modelMessages, messageEnds)
	}
	state, err := agent.State()
	if err != nil || len(state.Messages) != len(history)+1 {
		t.Fatalf("continued state/error = %#v/%v", state, err)
	}
}

func TestAgent_failedContinueRestoresIdleState(t *testing.T) {
	agent := newRunControlAgent(t, staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}))
	err := agent.Continue(context.Background())
	if !errors.Is(err, ErrEmptyContinuationContext) {
		t.Fatalf("Continue() error = %v, want ErrEmptyContinuationContext", err)
	}
	state, stateError := agent.State()
	if stateError != nil || state.IsStreaming || len(state.Messages) != 1 {
		t.Fatalf("state after failed Continue = %#v/%v", state, stateError)
	}
	if tail := assistantMessageAt(t, state.Messages, 0); tail.StopReason != StopReasonError {
		t.Fatalf("failed Continue tail = %#v, want error", tail)
	}
}
