package agent

import "slices"

// AgentContext 表示传递给 Agent Loop 的上下文快照。
type AgentContext struct {
	SystemPrompt string
	Messages     []Message
}

// WithMessages 返回追加消息后的独立上下文快照。
//
// 原有 AgentContext 及其 Messages slice 不会被修改。
func (context AgentContext) WithMessages(messages ...Message) AgentContext {
	next := context
	next.Messages = slices.Clone(context.Messages)
	next.Messages = append(next.Messages, messages...)

	return next
}
