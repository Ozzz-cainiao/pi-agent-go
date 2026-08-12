package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRunAgentLoop_returnsTypedError_whenStreamFuncNil(t *testing.T) {
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{},
		nil,
	)

	if !errors.Is(err, ErrNilStreamFunc) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrNilStreamFunc", err)
	}
	var configError *LoopConfigError
	if !errors.As(err, &configError) {
		t.Fatalf("RunAgentLoop() error type = %T, want *LoopConfigError", err)
	}
}

func TestRunAgentLoop_rejectsNegativeMaxTurns(t *testing.T) {
	streamCalled := false
	config := LoopConfig{
		MaxTurns: -1,
		Stream: func(
			context.Context,
			AgentContext,
			AssistantMessageEventSink,
		) (AssistantMessage, error) {
			streamCalled = true

			return AssistantMessage{}, nil
		},
	}

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		config,
		nil,
	)

	if !errors.Is(err, ErrInvalidLoopConfig) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrInvalidLoopConfig", err)
	}
	if streamCalled {
		t.Fatal("RunAgentLoop() called StreamFunc for invalid config")
	}
}

func TestRunAgentLoop_usesSafeDefaultMaxTurns(t *testing.T) {
	modelCalls := 0
	config := LoopConfig{
		Stream: func(
			context.Context,
			AgentContext,
			AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelCalls++

			return AssistantMessage{
				Content: []AssistantContent{
					ToolCall{ID: "call", Name: "qa_search"},
				},
				StopReason: StopReasonToolUse,
			}, nil
		},
	}

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{stubTool{}}},
		config,
		nil,
	)

	if !errors.Is(err, ErrMaxTurnsExceeded) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrMaxTurnsExceeded", err)
	}
	if modelCalls != DefaultMaxTurns {
		t.Fatalf("StreamFunc call count = %d, want %d", modelCalls, DefaultMaxTurns)
	}
}

func TestRunAgentLoop_usesInjectedClockForToolResult(t *testing.T) {
	fixed := time.Date(2026, time.August, 12, 7, 30, 0, 0, time.UTC)
	modelCalls := 0
	config := LoopConfig{
		Clock: func() time.Time {
			return fixed
		},
		Stream: func(
			context.Context,
			AgentContext,
			AssistantMessageEventSink,
		) (AssistantMessage, error) {
			modelCalls++
			if modelCalls == 1 {
				return AssistantMessage{
					Content: []AssistantContent{
						ToolCall{ID: "call", Name: "qa_search"},
					},
					StopReason: StopReasonToolUse,
				}, nil
			}

			return AssistantMessage{StopReason: StopReasonStop}, nil
		},
	}

	messages, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{stubTool{}}},
		config,
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	result, ok := messages[1].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[1] type = %T, want ToolResultMessage", messages[1])
	}
	if result.Timestamp != fixed.UnixMilli() {
		t.Fatalf("tool result timestamp = %d, want %d", result.Timestamp, fixed.UnixMilli())
	}
}

func TestContinueAgentLoop_rejectsEmptyHistory(t *testing.T) {
	streamCalled := false
	eventCalled := false
	_, err := ContinueAgentLoop(
		context.Background(),
		AgentContext{},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			streamCalled = true
			return AssistantMessage{}, nil
		}},
		func(AgentEvent) error {
			eventCalled = true
			return nil
		},
	)

	var typed *ContinuationError
	if !errors.As(err, &typed) || !errors.Is(err, ErrEmptyContinuationContext) {
		t.Fatalf("ContinueAgentLoop() error = %v, want typed empty-context error", err)
	}
	if streamCalled || eventCalled {
		t.Fatal("ContinueAgentLoop() started work for empty history")
	}
}

func TestContinueAgentLoop_rejectsAssistantTail(t *testing.T) {
	streamCalled := false
	_, err := ContinueAgentLoop(
		context.Background(),
		AgentContext{Messages: []AgentMessage{AssistantMessage{StopReason: StopReasonStop}}},
		LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
			streamCalled = true
			return AssistantMessage{}, nil
		}},
		nil,
	)

	if !errors.Is(err, ErrAssistantContinuation) || !errors.Is(err, ErrInvalidContinuation) {
		t.Fatalf("ContinueAgentLoop() error = %v, want assistant-tail error", err)
	}
	if streamCalled {
		t.Fatal("ContinueAgentLoop() called Stream after assistant tail")
	}
}

func TestContinueAgentLoop_usesHistoryWithoutReEmittingIt(t *testing.T) {
	history := []AgentMessage{
		UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
		AssistantMessage{
			Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
			StopReason: StopReasonToolUse,
		},
		ToolResultMessage{
			ToolCallID: "call-1",
			ToolName:   "qa_search",
			Content:    []ToolResultContent{TextContent{Text: "结果"}},
		},
	}
	original := AgentContext{SystemPrompt: "系统", Messages: history}
	wantOriginal := AgentContext{
		SystemPrompt: "系统",
		Messages: []AgentMessage{
			UserMessage{Content: []UserContent{TextContent{Text: "查询"}}},
			AssistantMessage{
				Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
				StopReason: StopReasonToolUse,
			},
			ToolResultMessage{
				ToolCallID: "call-1", ToolName: "qa_search",
				Content: []ToolResultContent{TextContent{Text: "结果"}},
			},
		},
	}
	response := AssistantMessage{
		Content:    []AssistantContent{TextContent{Text: "最终回答"}},
		StopReason: StopReasonStop,
	}
	var modelMessages []AgentMessage
	var messageEnds []AgentMessage

	got, err := ContinueAgentLoop(
		context.Background(),
		original,
		LoopConfig{Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
			modelMessages = modelContext.Messages
			return response, nil
		}},
		func(event AgentEvent) error {
			if end, ok := event.(AgentMessageEndEvent); ok {
				messageEnds = append(messageEnds, end.Message)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ContinueAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, []AgentMessage{response}) {
		t.Fatalf("new messages = %#v, want only response", got)
	}
	if !reflect.DeepEqual(modelMessages, history) {
		t.Fatalf("Model messages = %#v, want full existing history", modelMessages)
	}
	if !reflect.DeepEqual(messageEnds, []AgentMessage{response}) {
		t.Fatalf("message_end events = %#v, want only new response", messageEnds)
	}
	if !reflect.DeepEqual(original, wantOriginal) {
		t.Fatal("ContinueAgentLoop() modified input context")
	}
}

func TestRunAgentLoop_appliesTransformBeforeConvertWithoutMutatingInput(t *testing.T) {
	originalMessage := UserMessage{Content: []UserContent{TextContent{Text: "原始"}}}
	initial := AgentContext{Messages: []AgentMessage{originalMessage}}
	var order []string

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		initial,
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				text := userTextAt(t, modelContext.Messages, 0)
				if text.Text != "已转换" {
					t.Fatalf("Model text = %q, want transformed value", text.Text)
				}
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			TransformContext: func(_ context.Context, messages []AgentMessage) ([]AgentMessage, error) {
				order = append(order, "transform")
				user := userMessageAt(t, messages, 0)
				user.Content[0] = TextContent{Text: "已转换"}
				messages[0] = user
				return messages, nil
			},
			ConvertToLLM: func(_ context.Context, messages []AgentMessage) ([]Message, error) {
				order = append(order, "convert")
				return []Message{userMessageAt(t, messages, 0)}, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"transform", "convert"}) {
		t.Fatalf("hook order = %#v, want transform then convert", order)
	}
	if !reflect.DeepEqual(initial.Messages, []AgentMessage{originalMessage}) {
		t.Fatal("context hooks modified input context")
	}
}

func TestRunAgentLoop_usesPrepareNextTurnSnapshot(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "原始"}}}
	toolRequest := AssistantMessage{
		Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
		StopReason: StopReasonToolUse,
	}
	final := AssistantMessage{StopReason: StopReasonStop}
	modelCall := 0
	prepareCall := 0

	_, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{SystemPrompt: "first", Tools: []Tool{stubTool{}}},
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				modelCall++
				if modelCall == 1 {
					return toolRequest, nil
				}
				if modelContext.SystemPrompt != "second" {
					t.Fatalf("second SystemPrompt = %q, want second", modelContext.SystemPrompt)
				}
				text := userTextAt(t, modelContext.Messages, 0)
				if text.Text != "原始" {
					t.Fatalf("second prompt text = %q, snapshot mutation leaked", text.Text)
				}
				return final, nil
			},
			PrepareNextTurn: func(_ context.Context, turn PrepareNextTurnContext) (NextTurnUpdate, error) {
				prepareCall++
				if prepareCall > 1 {
					return NextTurnUpdate{}, nil
				}
				user := userMessageAt(t, turn.Context.Messages, 0)
				user.Content[0] = TextContent{Text: "篡改快照"}
				turn.Context.Messages[0] = user
				next := AgentContext{
					SystemPrompt: "second",
					Messages:     append([]AgentMessage(nil), turn.NewMessages...),
					Tools:        turn.Context.Tools,
				}
				next.Messages[0] = prompt
				return NextTurnUpdate{Context: &next}, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCall != 2 || prepareCall != 2 {
		t.Fatalf("calls = model:%d prepare:%d, want 2/2", modelCall, prepareCall)
	}
	text, ok := prompt.Content[0].(TextContent)
	if !ok {
		t.Fatalf("input content type = %T, want TextContent", prompt.Content[0])
	}
	if text.Text != "原始" {
		t.Fatalf("input prompt = %q, want unchanged", text.Text)
	}
}

func userMessageAt(t *testing.T, messages []AgentMessage, index int) UserMessage {
	t.Helper()
	message, ok := messages[index].(UserMessage)
	if !ok {
		t.Fatalf("message[%d] type = %T, want UserMessage", index, messages[index])
	}
	return message
}

func userTextAt(t *testing.T, messages []AgentMessage, index int) TextContent {
	t.Helper()
	message := userMessageAt(t, messages, index)
	content, ok := message.Content[0].(TextContent)
	if !ok {
		t.Fatalf("message[%d] content type = %T, want TextContent", index, message.Content[0])
	}
	return content
}

type countingTool struct {
	calls int
}

func (tool *countingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "qa_search"}
}

func (tool *countingTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	tool.calls++

	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: "不应该执行"},
		},
	}, nil
}

func TestRunAgentLoop_terminalStopReasonDoesNotExecuteTool(t *testing.T) {
	reasons := []StopReason{
		StopReasonError,
		StopReasonAborted,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			// 准备
			tool := &countingTool{}
			modelCalls := 0

			response := AssistantMessage{
				Content: []AssistantContent{
					ToolCall{
						ID:   "call-1",
						Name: "qa_search",
					},
				},
				StopReason: reason,
			}

			streamFn := StreamFunc(func(
				_ context.Context,
				_ AgentContext,
				_ AssistantMessageEventSink,
			) (AssistantMessage, error) {
				modelCalls++
				if modelCalls > 1 {
					return AssistantMessage{}, errors.New("unexpected Model call")
				}

				return response, nil
			})

			// 执行
			got, err := RunAgentLoop(
				context.Background(),
				nil,
				AgentContext{Tools: []Tool{tool}},
				LoopConfig{Stream: streamFn},
				nil,
			)
			// 验证
			if err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
			if modelCalls != 1 {
				t.Fatalf("Model call count = %d, want 1", modelCalls)
			}
			if tool.calls != 0 {
				t.Fatalf("Tool call count = %d, want 0", tool.calls)
			}
			if len(got) != 1 {
				t.Fatalf("new message count = %d, want 1", len(got))
			}
		})
	}
}

func TestRunAgentLoop_doesNotExecuteTruncatedToolCall(t *testing.T) {
	// 准备
	tool := &countingTool{}
	modelCalls := 0

	truncatedResponse := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "可能被截断的参数",
				},
			},
		},
		StopReason: StopReasonLength,
	}

	finalResponse := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "工具参数被截断，请重试。"},
		},
		StopReason: StopReasonStop,
	}

	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++

		switch modelCalls {
		case 1:
			return truncatedResponse, nil
		case 2:
			return finalResponse, nil
		default:
			return AssistantMessage{}, errors.New("unexpected Model call")
		}
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{Stream: streamFn},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if tool.calls != 0 {
		t.Fatalf("Tool call count = %d, want 0", tool.calls)
	}
	if modelCalls != 2 {
		t.Fatalf("Model call count = %d, want 2", modelCalls)
	}
	if len(got) != 3 {
		t.Fatalf("new message count = %d, want 3", len(got))
	}

	result, ok := got[1].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[1] type = %T, want ToolResultMessage", got[1])
	}
	if !result.IsError {
		t.Fatal("truncated ToolResultMessage IsError = false, want true")
	}

	wantContent := []ToolResultContent{
		TextContent{
			Text: `Tool call "qa_search" was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.`,
		},
	}
	if !reflect.DeepEqual(result.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", result.Content, wantContent)
	}
}

func TestRunAgentLoop_callsModelWithHistoryAndPrompt(t *testing.T) {
	// 准备
	history := UserMessage{
		Content: []UserContent{
			TextContent{Text: "历史问题"},
		},
		Timestamp: 1,
	}
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "本轮问题"},
		},
		Timestamp: 2,
	}
	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模型回答"},
		},
		StopReason: StopReasonStop,
		Timestamp:  3,
	}

	initial := AgentContext{
		SystemPrompt: "你是一个助手。",
		Messages:     []AgentMessage{history},
		Tools:        []Tool{stubTool{}},
	}

	var receivedContext AgentContext
	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		receivedContext = agentContext

		return response, nil
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		initial,
		LoopConfig{Stream: streamFn},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if len(initial.Messages) != 1 {
		t.Fatalf("initial message count = %d, want 1", len(initial.Messages))
	}
	if len(receivedContext.Messages) != 2 {
		t.Fatalf(
			"model context message count = %d, want 2",
			len(receivedContext.Messages),
		)
	}

	want := []AgentMessage{prompt, response}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunAgentLoop() = %#v, want %#v", got, want)
	}
}

func TestRunAgentLoop_doesNotCallModelWhenContextCanceled(t *testing.T) {
	// 准备
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "本轮问题"},
		},
		Timestamp: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	modelCalled := false
	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalled = true

		return AssistantMessage{}, nil
	})

	// 执行
	got, err := RunAgentLoop(
		ctx,
		[]AgentMessage{prompt},
		AgentContext{},
		LoopConfig{Stream: streamFn},
		nil,
	)

	// 验证
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAgentLoop() error = %v, want context.Canceled", err)
	}
	if modelCalled {
		t.Fatal("RunAgentLoop() called Model after context cancellation")
	}

	want := []AgentMessage{prompt}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunAgentLoop() = %#v, want %#v", got, want)
	}
}

func TestRunAgentLoop_forwardsAssistantEvents(t *testing.T) {
	// 准备
	partial := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模"},
		},
		StopReason: StopReasonPending,
	}

	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模型回答"},
		},
		StopReason: StopReasonStop,
	}

	wantEvents := []AssistantMessageEvent{
		AssistantTextDeltaEvent{
			ContentIndex: 0,
			Delta:        "模",
			Partial:      partial,
		},
	}

	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		emit AssistantMessageEventSink,
	) (AssistantMessage, error) {
		for _, event := range wantEvents {
			if err := emit(event); err != nil {
				return AssistantMessage{}, err
			}
		}

		return response, nil
	})

	var gotEvents []AssistantMessageEvent
	emit := AgentEventSink(func(event AgentEvent) error {
		update, ok := event.(AgentMessageUpdateEvent)
		if ok {
			gotEvents = append(gotEvents, update.AssistantEvent)
		}

		return nil
	})

	// 执行
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: streamFn},
		emit,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("events = %#v, want %#v", gotEvents, wantEvents)
	}
}

func TestRunAgentLoop_executesToolCallAndContinuesModel(t *testing.T) {
	// 准备
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "查询 Agent Loop"},
		},
		Timestamp: 1,
	}

	toolRequest := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "Agent Loop",
				},
			},
		},
		StopReason: StopReasonToolUse,
		Timestamp:  2,
	}

	finalResponse := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "Agent Loop 负责协调模型和工具。"},
		},
		StopReason: StopReasonStop,
		Timestamp:  4,
	}

	modelCalls := 0
	var secondContext AgentContext

	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++

		switch modelCalls {
		case 1:
			return toolRequest, nil
		case 2:
			secondContext = agentContext

			return finalResponse, nil
		default:
			return AssistantMessage{}, errors.New("unexpected Model call")
		}
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{
			Tools: []Tool{stubTool{}},
		},
		LoopConfig{Stream: streamFn},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("Model call count = %d, want 2", modelCalls)
	}
	if len(secondContext.Messages) != 3 {
		t.Fatalf(
			"second Model context message count = %d, want 3",
			len(secondContext.Messages),
		)
	}
	if len(got) != 4 {
		t.Fatalf("new message count = %d, want 4", len(got))
	}

	toolResult, ok := got[2].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[2] type = %T, want ToolResultMessage", got[2])
	}
	if toolResult.ToolCallID != "call-1" {
		t.Fatalf(
			"ToolCallID = %q, want %q",
			toolResult.ToolCallID,
			"call-1",
		)
	}
	if toolResult.IsError {
		t.Fatal("tool result IsError = true, want false")
	}
	if toolResult.Timestamp <= 0 {
		t.Fatalf("tool result Timestamp = %d, want positive value", toolResult.Timestamp)
	}
	if !reflect.DeepEqual(got[3], finalResponse) {
		t.Fatalf("final message = %#v, want %#v", got[3], finalResponse)
	}
}

func TestRunAgentLoop_handlesEveryStopReason(t *testing.T) {
	tests := []struct {
		name       string
		reason     StopReason
		withTool   bool
		modelCalls int
		toolCalls  int
	}{
		{name: "pending", reason: StopReasonPending, modelCalls: 1},
		{name: "stop", reason: StopReasonStop, modelCalls: 1},
		{name: "length", reason: StopReasonLength, modelCalls: 1},
		{name: "toolUse", reason: StopReasonToolUse, withTool: true, modelCalls: 2, toolCalls: 1},
		{name: "error", reason: StopReasonError, withTool: true, modelCalls: 1},
		{name: "aborted", reason: StopReasonAborted, withTool: true, modelCalls: 1},
		{name: "deferred", reason: StopReasonDeferred, modelCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := &countingTool{}
			calls := 0
			stream := func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				calls++
				if calls > 1 {
					return AssistantMessage{StopReason: StopReasonStop}, nil
				}
				response := AssistantMessage{StopReason: test.reason}
				if test.withTool {
					response.Content = []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}}
				}
				return response, nil
			}

			_, err := RunAgentLoop(context.Background(), nil, AgentContext{Tools: []Tool{tool}}, LoopConfig{Stream: stream}, nil)
			if err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
			if calls != test.modelCalls || tool.calls != test.toolCalls {
				t.Fatalf("calls = model:%d tool:%d, want %d/%d", calls, tool.calls, test.modelCalls, test.toolCalls)
			}
		})
	}
}

func TestRunAgentLoop_returnsTypedErrorAtConfiguredMaxTurns(t *testing.T) {
	modelCalls := 0
	tool := &countingTool{}
	var events []AgentEvent
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{
			MaxTurns: 3,
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{
					Content:    []AssistantContent{ToolCall{ID: "call", Name: "qa_search"}},
					StopReason: StopReasonToolUse,
				}, nil
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)

	var turnError *MaxTurnsError
	if !errors.As(err, &turnError) || !errors.Is(err, ErrMaxTurnsExceeded) {
		t.Fatalf("RunAgentLoop() error = %v, want typed max turns error", err)
	}
	if turnError.MaxTurns != 3 || modelCalls != 3 || tool.calls != 3 {
		t.Fatalf("limit/calls = %d/%d/%d, want 3/3/3", turnError.MaxTurns, modelCalls, tool.calls)
	}
	if countEventKind(events, AgentEventTurnStart) != 3 || countEventKind(events, AgentEventTurnEnd) != 3 {
		t.Fatalf("turn events = start:%d end:%d, want 3/3", countEventKind(events, AgentEventTurnStart), countEventKind(events, AgentEventTurnEnd))
	}
}

func TestRunAgentLoop_shouldStopAfterCompletedTurn(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "开始"}}}
	tool := &countingTool{}
	modelCalls := 0
	steeringPolls := 0
	followUpPolls := 0
	var callback ShouldStopAfterTurnContext
	var events []AgentEvent

	messages, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{
			Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				return AssistantMessage{
					Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
					StopReason: StopReasonToolUse,
				}, nil
			},
			ShouldStopAfterTurn: func(_ context.Context, turn ShouldStopAfterTurnContext) (bool, error) {
				callback = turn
				return true, nil
			},
			GetSteeringMessages: func(context.Context) ([]AgentMessage, error) {
				steeringPolls++
				return nil, nil
			},
			GetFollowUpMessages: func(context.Context) ([]AgentMessage, error) {
				followUpPolls++
				return []AgentMessage{prompt}, nil
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 1 || tool.calls != 1 || steeringPolls != 1 || followUpPolls != 0 {
		t.Fatalf("calls = model:%d tool:%d steering:%d follow-up:%d, want 1/1/1/0", modelCalls, tool.calls, steeringPolls, followUpPolls)
	}
	if len(callback.ToolResults) != 1 || len(callback.Context.Messages) != 3 || !reflect.DeepEqual(callback.NewMessages, messages) {
		t.Fatalf("stop callback snapshot = %#v, messages = %#v", callback, messages)
	}
	wantTail := []AgentEventKind{AgentEventToolExecutionEnd, AgentEventMessageStart, AgentEventMessageEnd, AgentEventTurnEnd, AgentEventAgentEnd}
	gotKinds := agentEventKinds(events)
	if !reflect.DeepEqual(gotKinds[len(gotKinds)-len(wantTail):], wantTail) {
		t.Fatalf("event tail = %#v, want %#v", gotKinds, wantTail)
	}
}

func TestRunAgentLoop_returnsTypedTurnControlHookError(t *testing.T) {
	hookError := errors.New("stop hook failed")
	var events []AgentEvent
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{
			Stream: staticAssistantStream(AssistantMessage{StopReason: StopReasonStop}),
			ShouldStopAfterTurn: func(context.Context, ShouldStopAfterTurnContext) (bool, error) {
				return false, hookError
			},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			return nil
		},
	)

	var typed *TurnControlHookError
	if !errors.As(err, &typed) || !errors.Is(err, ErrTurnControlHook) || !errors.Is(err, hookError) {
		t.Fatalf("RunAgentLoop() error = %v, want typed hook error", err)
	}
	if typed.Hook != "ShouldStopAfterTurn" {
		t.Fatalf("hook name = %q, want ShouldStopAfterTurn", typed.Hook)
	}
	wantTail := []AgentEventKind{AgentEventTurnEnd, AgentEventAgentEnd}
	gotKinds := agentEventKinds(events)
	if !reflect.DeepEqual(gotKinds[len(gotKinds)-len(wantTail):], wantTail) {
		t.Fatalf("event kinds = %#v, want closed lifecycle", gotKinds)
	}
}

func countEventKind(events []AgentEvent, kind AgentEventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind() == kind {
			count++
		}
	}
	return count
}
