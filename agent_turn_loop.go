package agent

import (
	"context"
	"fmt"
)

type loopState struct {
	current  AgentContext
	messages []AgentMessage
}

type completedTurn struct {
	response    AssistantMessage
	toolResults []ToolResultMessage
	continues   bool
}

func runAgentTurns(
	ctx context.Context,
	prompts []AgentMessage,
	config LoopConfig,
	events agentEventEmitter,
	state *loopState,
) error {
	runtime, err := config.runtime()
	if err != nil {
		return fmt.Errorf("run agent loop: %w", err)
	}
	pending, err := getQueuedMessages(ctx, runtime.getSteeringMessages, "GetSteeringMessages")
	if err != nil {
		return err
	}

	for turn := 0; ; turn++ {
		completed, turnError := runSingleTurn(ctx, turn, prompts, pending, runtime, events, state)
		if turnError != nil {
			return turnError
		}
		if isErrorTerminal(completed.response.StopReason) {
			return nil
		}
		if err := prepareNextTurn(ctx, runtime.prepareNextTurn, state, completed.response, completed.toolResults); err != nil {
			return err
		}
		stop, err := shouldStopAfterTurn(ctx, runtime.shouldStopAfterTurn, state, completed.response, completed.toolResults)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		pending, err = getQueuedMessages(ctx, runtime.getSteeringMessages, "GetSteeringMessages")
		if err != nil {
			return err
		}
		if completed.continues || len(pending) > 0 {
			continue
		}
		pending, err = getQueuedMessages(ctx, runtime.getFollowUpMessages, "GetFollowUpMessages")
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
	}
}

func runSingleTurn(
	ctx context.Context,
	turn int,
	prompts []AgentMessage,
	pending []AgentMessage,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
) (completedTurn, error) {
	if err := validateTurn(ctx, turn, runtime.maxTurns); err != nil {
		return completedTurn{}, err
	}
	if err := events.emit(AgentTurnStartEvent{}); err != nil {
		return completedTurn{}, err
	}
	if turn == 0 {
		if err := emitPromptMessages(events, prompts); err != nil {
			return completedTurn{}, err
		}
	}
	if err := injectQueuedMessages(events, state, pending); err != nil {
		return completedTurn{}, err
	}
	response, err := streamAssistantTurn(ctx, runtime, events, state)
	if err != nil {
		return completedTurn{}, err
	}
	toolResults, continues, err := completeTurn(ctx, runtime, events, state, response)
	if err != nil {
		return completedTurn{}, err
	}
	if err := events.emit(AgentTurnEndEvent{Message: response, ToolResults: toolResults}); err != nil {
		return completedTurn{}, err
	}
	return completedTurn{response: response, toolResults: toolResults, continues: continues}, nil
}

func isErrorTerminal(reason StopReason) bool {
	return reason == StopReasonError || reason == StopReasonAborted
}

func validateTurn(ctx context.Context, turn, maxTurns int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run agent loop: %w", err)
	}
	if turn >= maxTurns {
		return &MaxTurnsError{MaxTurns: maxTurns}
	}
	return nil
}

func emitPromptMessages(events agentEventEmitter, prompts []AgentMessage) error {
	for _, message := range prompts {
		if err := events.emit(AgentMessageStartEvent{Message: message}); err != nil {
			return err
		}
		if err := events.emit(AgentMessageEndEvent{Message: message}); err != nil {
			return err
		}
	}
	return nil
}

func streamAssistantTurn(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
) (AssistantMessage, error) {
	modelContext, err := state.current.toLLM(
		ctx,
		runtime.transformContext,
		runtime.convertToLLM,
	)
	if err != nil {
		return AssistantMessage{}, fmt.Errorf("convert messages to llm: %w", err)
	}
	lifecycle := assistantLifecycle{events: events}
	response, err := runtime.stream(ctx, modelContext, lifecycle.sink())
	if err != nil {
		return AssistantMessage{}, fmt.Errorf("stream assistant response: %w", err)
	}
	if err := lifecycle.finish(response); err != nil {
		return AssistantMessage{}, err
	}
	state.messages = append(state.messages, response)
	state.current = state.current.WithMessages(response)
	return response, nil
}
