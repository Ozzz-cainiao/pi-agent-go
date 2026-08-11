package agent

import (
	"context"
	"errors"
	"fmt"
)

// Prompt 启动一次低层 Agent Loop，并等待 subscriber 全部完成。
func (agent *Agent) Prompt(ctx context.Context, prompts ...AgentMessage) error {
	return agent.executeRun(ctx, func(
		runContext context.Context,
		state AgentState,
		sink AgentEventSink,
	) error {
		clonedPrompts, err := cloneAgentMessages(prompts)
		if err != nil {
			return fmt.Errorf("clone agent prompts: %w", err)
		}
		_, err = RunAgentLoop(
			runContext,
			clonedPrompts,
			agentContextFromState(state),
			agent.loopConfigForRun(false),
			sink,
		)
		return err
	})
}

// Continue 从当前 transcript 尾部继续低层 Agent Loop。
func (agent *Agent) Continue(ctx context.Context) error {
	return agent.executeRun(ctx, func(
		runContext context.Context,
		state AgentState,
		sink AgentEventSink,
	) error {
		return agent.continueFromState(runContext, state, sink)
	})
}

func (agent *Agent) continueFromState(
	ctx context.Context,
	state AgentState,
	sink AgentEventSink,
) error {
	if len(state.Messages) > 0 {
		last := state.Messages[len(state.Messages)-1]
		switch last.(type) {
		case AssistantMessage, *AssistantMessage:
			return agent.continueFromAssistant(ctx, state, sink)
		}
	}
	_, err := ContinueAgentLoop(
		ctx,
		agentContextFromState(state),
		agent.loopConfigForRun(false),
		sink,
	)
	return err
}

func (agent *Agent) continueFromAssistant(
	ctx context.Context,
	state AgentState,
	sink AgentEventSink,
) error {
	steering, err := agent.drainQueue(agentSteeringQueue)
	if err != nil {
		return err
	}
	if len(steering) > 0 {
		return agent.runQueuedPrompt(ctx, state, steering, true, sink)
	}
	followUp, err := agent.drainQueue(agentFollowUpQueue)
	if err != nil {
		return err
	}
	if len(followUp) > 0 {
		return agent.runQueuedPrompt(ctx, state, followUp, false, sink)
	}
	return &ContinuationError{Cause: ErrAssistantContinuation}
}

func (agent *Agent) runQueuedPrompt(
	ctx context.Context,
	state AgentState,
	prompts []AgentMessage,
	skipInitialSteering bool,
	sink AgentEventSink,
) error {
	_, err := RunAgentLoop(
		ctx,
		prompts,
		agentContextFromState(state),
		agent.loopConfigForRun(skipInitialSteering),
		sink,
	)
	return err
}

type agentRunExecutor func(context.Context, AgentState, AgentEventSink) error

func (agent *Agent) executeRun(
	ctx context.Context,
	executor agentRunExecutor,
) (result error) {
	runContext, cancel := context.WithCancel(ctx)
	reply, err := agent.execute(agentStateCommand{
		operation: agentStateBeginRun,
		runDone:   make(chan struct{}),
		cancel:    cancel,
	})
	if err != nil {
		cancel()
		return err
	}
	subscriberErrors := make([]error, 0)
	dispatcher := agentRunEventDispatcher{
		agent: agent, ctx: runContext, failures: &subscriberErrors,
	}
	defer func() {
		_, finishError := agent.execute(agentStateCommand{operation: agentStateFinishRun})
		cancel()
		result = errors.Join(result, errors.Join(subscriberErrors...), finishError)
	}()
	runError := executor(runContext, reply.state, dispatcher.handle)
	lifecycleError := dispatcher.complete(runError, reply.state.Model)
	return errors.Join(runError, lifecycleError)
}

func agentContextFromState(state AgentState) AgentContext {
	return AgentContext{
		SystemPrompt: state.SystemPrompt,
		Messages:     state.Messages,
		Tools:        state.Tools,
	}
}

// WaitForIdle 等待调用时的 run settled；关闭发起后则等待 owner 完全停止。
func (agent *Agent) WaitForIdle(ctx context.Context) error {
	if agent.closing.Load() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for agent close: %w", ctx.Err())
		case <-agent.done:
			return nil
		}
	}
	reply, err := agent.execute(agentStateCommand{operation: agentStateCurrentIdle})
	if err != nil {
		return err
	}
	if reply.idle == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for agent idle: %w", ctx.Err())
	case <-reply.idle:
		return nil
	}
}
