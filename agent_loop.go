package agent

import (
	"context"
	"fmt"
	"slices"
)

// RunAgentLoop 使用本轮消息调用 Model，并返回本轮新增的消息。
func RunAgentLoop(
	ctx context.Context,
	prompts []Message,
	initial AgentContext,
	streamFn StreamFunc,
) ([]Message, error) {
	/*
		RunAgentLoop(
			在这个 ctx 生命周期中,
			处理这些 prompts,
			基于 initial 上下文,
			使用 streamFn 调用模型,
		)
		ctx       控制什么时候停止
		prompts   表示本轮新增消息
		initial   提供历史、system prompt 和 tools
		streamFn  决定具体如何调用模型
	*/
	newMessages := slices.Clone(prompts)

	// 在调用模型前判断本次 Agent 的 Loop 是否已经被取消或超时
	if err := ctx.Err(); err != nil {
		return newMessages, fmt.Errorf("run agent loop: %w", err)
	}

	current := initial.WithMessages(prompts...)

	// ctx 在这里传递给 model
	response, err := streamFn(
		ctx,
		current,
		func(AssistantMessageEvent) error {
			return nil
		},
	)
	if err != nil {
		return newMessages, fmt.Errorf("stream assistant response: %w", err)
	}

	// 即使 Model 没有执行，本轮用户输入仍然已经被 Agent Loop 接收。因此返回
	newMessages = append(newMessages, response)

	return newMessages, nil
}
