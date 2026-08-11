package agent

import (
	"context"
	"errors"
	"fmt"
)

// Prompt 启动一次低层 Agent Loop，并等待 subscriber 全部完成。
func (agent *Agent) Prompt(ctx context.Context, prompts ...AgentMessage) error {
	clonedPrompts, err := cloneAgentMessages(prompts)
	if err != nil {
		return fmt.Errorf("clone agent prompts: %w", err)
	}
	runDone := make(chan struct{})
	reply, err := agent.execute(agentStateCommand{
		operation: agentStateBeginRun,
		runDone:   runDone,
	})
	if err != nil {
		return err
	}

	subscriberErrors := make([]error, 0)
	sink := func(event AgentEvent) error {
		failures, dispatchError := agent.dispatchEvent(ctx, event)
		subscriberErrors = append(subscriberErrors, failures...)
		return dispatchError
	}
	initial := AgentContext{
		SystemPrompt: reply.state.SystemPrompt,
		Messages:     reply.state.Messages,
		Tools:        reply.state.Tools,
	}
	_, runError := RunAgentLoop(ctx, clonedPrompts, initial, agent.loopConfig, sink)
	_, finishError := agent.execute(agentStateCommand{operation: agentStateFinishRun})
	return errors.Join(runError, errors.Join(subscriberErrors...), finishError)
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
