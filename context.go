package agent

import (
	"context"
	"slices"
)

// AgentContext 表示传递给 Agent Loop 的上下文快照。
type AgentContext struct {
	SystemPrompt string
	Messages     []AgentMessage
	Tools        []Tool
}

// WithMessages 返回追加消息后的独立上下文快照。
//
// 原有 AgentContext 及其 Messages slice 不会被修改。
func (current AgentContext) WithMessages(messages ...AgentMessage) AgentContext {
	next := current
	next.Messages = slices.Clone(current.Messages)
	next.Messages = append(next.Messages, messages...)
	next.Tools = slices.Clone(current.Tools)

	return next
}

func (current AgentContext) toLLM(
	ctx context.Context,
	convert ConvertToLLMFunc,
) (AgentContext, error) {
	messages, err := convert(ctx, current.Messages)
	if err != nil {
		return AgentContext{}, err
	}

	next := current
	next.Messages = make([]AgentMessage, len(messages))
	for index, message := range messages {
		next.Messages[index] = message
	}
	next.Tools = slices.Clone(current.Tools)

	return next, nil
}
