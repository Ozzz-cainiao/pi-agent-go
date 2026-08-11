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
			agent.loopConfig,
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
		_, err := ContinueAgentLoop(
			runContext,
			agentContextFromState(state),
			agent.loopConfig,
			sink,
		)
		return err
	})
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
	defer func() {
		_, finishError := agent.execute(agentStateCommand{operation: agentStateFinishRun})
		cancel()
		result = errors.Join(result, errors.Join(subscriberErrors...), finishError)
	}()
	sink := func(event AgentEvent) error {
		failures, dispatchError := agent.dispatchEvent(runContext, event)
		subscriberErrors = append(subscriberErrors, failures...)
		return dispatchError
	}
	return executor(runContext, reply.state, sink)
}

func agentContextFromState(state AgentState) AgentContext {
	return AgentContext{
		SystemPrompt: state.SystemPrompt,
		Messages:     state.Messages,
		Tools:        state.Tools,
	}
}

// WaitForIdle 等待调用时正在执行的 run 完全 settled。
func (agent *Agent) WaitForIdle(ctx context.Context) error {
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
