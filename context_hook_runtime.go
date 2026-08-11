package agent

import (
	"context"
	"fmt"
)

func prepareNextTurn(
	ctx context.Context,
	hook PrepareNextTurnFunc,
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) error {
	if hook == nil {
		return nil
	}
	snapshot, err := newPrepareNextTurnContext(state, message, toolResults)
	if err != nil {
		return fmt.Errorf("snapshot prepare next turn context: %w", err)
	}
	update, err := hook(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("prepare next turn: %w", err)
	}
	if update.Context == nil {
		return nil
	}
	next, err := cloneAgentContext(*update.Context)
	if err != nil {
		return fmt.Errorf("snapshot next turn update: %w", err)
	}
	state.current = next
	return nil
}

func newPrepareNextTurnContext(
	state *loopState,
	message AssistantMessage,
	toolResults []ToolResultMessage,
) (PrepareNextTurnContext, error) {
	cloner := newSnapshotCloner()
	messageSnapshot, err := cloner.cloneAssistantMessage(message)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	resultSnapshots := make([]ToolResultMessage, len(toolResults))
	for index, result := range toolResults {
		resultSnapshots[index], err = cloner.cloneToolResultMessage(result)
		if err != nil {
			return PrepareNextTurnContext{}, err
		}
	}
	contextSnapshot, err := cloneAgentContext(state.current)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	newMessages, err := cloneAgentMessages(state.messages)
	if err != nil {
		return PrepareNextTurnContext{}, err
	}
	return PrepareNextTurnContext{
		Message: messageSnapshot, ToolResults: resultSnapshots,
		Context: contextSnapshot, NewMessages: newMessages,
	}, nil
}
