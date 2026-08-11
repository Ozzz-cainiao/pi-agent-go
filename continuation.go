package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

var (
	// ErrInvalidContinuation 表示无法从当前上下文继续 Agent Loop。
	ErrInvalidContinuation = errors.New("agent: invalid continuation")
	// ErrEmptyContinuationContext 表示继续执行时没有历史消息。
	ErrEmptyContinuationContext = errors.New("agent: empty continuation context")
	// ErrAssistantContinuation 表示不能从 Assistant 消息之后直接继续。
	ErrAssistantContinuation = errors.New("agent: cannot continue after assistant message")
)

// ContinuationError 描述 ContinueAgentLoop 的输入错误。
type ContinuationError struct{ Cause error }

func (continuationError *ContinuationError) Error() string {
	return fmt.Sprintf("agent: cannot continue loop: %v", continuationError.Cause)
}

func (continuationError *ContinuationError) Unwrap() []error {
	return []error{ErrInvalidContinuation, continuationError.Cause}
}

// ContinueAgentLoop 从已有历史继续执行，只返回本次新增消息。
func ContinueAgentLoop(
	ctx context.Context,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) ([]AgentMessage, error) {
	if err := validateContinuation(initial); err != nil {
		return nil, err
	}
	current := initial
	current.Messages = slices.Clone(initial.Messages)
	current.Tools = slices.Clone(initial.Tools)
	state := loopState{current: current, messages: []AgentMessage{}}
	return runLoopLifecycle(ctx, nil, config, sink, &state)
}

func validateContinuation(context AgentContext) error {
	if len(context.Messages) == 0 {
		return &ContinuationError{Cause: ErrEmptyContinuationContext}
	}
	last := context.Messages[len(context.Messages)-1]
	switch last.(type) {
	case AssistantMessage, *AssistantMessage:
		return &ContinuationError{Cause: ErrAssistantContinuation}
	default:
		return nil
	}
}
