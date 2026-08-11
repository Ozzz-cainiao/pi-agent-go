package agent

import (
	"context"
	"errors"
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
