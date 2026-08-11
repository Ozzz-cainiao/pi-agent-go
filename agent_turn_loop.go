package agent

import (
	"context"
	"fmt"
)

type loopState struct {
	current  AgentContext
	messages []AgentMessage
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

	for turn := 0; ; turn++ {
		if err := validateTurn(ctx, turn, runtime.maxTurns); err != nil {
			return err
		}
		if err := events.emit(AgentTurnStartEvent{}); err != nil {
			return err
		}
		if turn == 0 {
			if err := emitPromptMessages(events, prompts); err != nil {
				return err
			}
		}

		response, err := streamAssistantTurn(ctx, runtime, events, state)
		if err != nil {
			return err
		}
		toolResults, continues, err := completeTurn(ctx, runtime, events, state, response)
		if err != nil {
			return err
		}
		if err := events.emit(AgentTurnEndEvent{
			Message: response, ToolResults: toolResults,
		}); err != nil {
			return err
		}
		if !continues {
			return nil
		}
	}
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
	modelContext, err := state.current.toLLM(ctx, runtime.convertToLLM)
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
