package agent

import (
	"context"
	"errors"
	"fmt"
)

// ErrTurnControlHook 表示轮次控制 Hook 执行失败。
var ErrTurnControlHook = errors.New("agent: turn control hook failed")

// TurnControlHookError 保留失败 Hook 的名称和原始错误。
type TurnControlHookError struct {
	Hook  string
	Cause error
}

// Error 返回轮次控制 Hook 的错误说明。
func (hookError *TurnControlHookError) Error() string {
	return fmt.Sprintf("agent: turn control hook %s failed: %v", hookError.Hook, hookError.Cause)
}

// Unwrap 同时暴露通用哨兵错误和原始错误。
func (hookError *TurnControlHookError) Unwrap() []error {
	return []error{ErrTurnControlHook, hookError.Cause}
}

func shouldStopAfterTurn(
	ctx context.Context,
	hook ShouldStopAfterTurnFunc,
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) (bool, error) {
	if hook == nil {
		return false, nil
	}
	snapshot, err := newPrepareNextTurnContext(state, message, toolResults)
	if err != nil {
		return false, fmt.Errorf("snapshot should stop after turn context: %w", err)
	}
	stop, err := hook(ctx, snapshot)
	if err != nil {
		return false, &TurnControlHookError{Hook: "ShouldStopAfterTurn", Cause: err}
	}
	return stop, nil
}

func getQueuedMessages(
	ctx context.Context,
	hook GetQueuedMessagesFunc,
	hookName string,
) ([]AgentMessage, error) {
	if hook == nil {
		return nil, nil
	}
	messages, err := hook(ctx)
	if err != nil {
		return nil, &TurnControlHookError{Hook: hookName, Cause: err}
	}
	snapshot, err := cloneAgentMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("snapshot queued messages from %s: %w", hookName, err)
	}
	return snapshot, nil
}

func injectQueuedMessages(
	events agentEventEmitter,
	state *loopState,
	messages []AgentMessage,
) error {
	for _, message := range messages {
		if err := emitMessageLifecycle(events, message); err != nil {
			return err
		}
		state.messages = append(state.messages, message)
		state.current = state.current.WithMessages(message)
	}
	return nil
}
