package agent

import "context"

// BeforeToolCallContext 是工具执行前 Hook 接收的上下文快照。
type BeforeToolCallContext struct {
	AssistantMessage AssistantMessage
	ToolCall         ToolCall
	Arguments        map[string]any
	Context          AgentContext
}

// BeforeToolCallResult 描述工具执行前的阻断决定。
type BeforeToolCallResult struct {
	Block     bool
	Reason    string
	Terminate bool
}

// BeforeToolCallFunc 在参数校验完成后、工具执行前运行。
type BeforeToolCallFunc func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error)

// AfterToolCallContext 是工具执行后 Hook 接收的上下文快照。
type AfterToolCallContext struct {
	AssistantMessage AssistantMessage
	ToolCall         ToolCall
	Arguments        map[string]any
	Result           ToolResult
	IsError          bool
	Context          AgentContext
}

// AfterToolCallResult 按字段覆盖工具执行结果；nil 字段保持原值。
type AfterToolCallResult struct {
	Content   []ToolResultContent
	Details   any
	Usage     *Usage
	IsError   *bool
	Terminate *bool
}

// AfterToolCallFunc 在工具执行完成后、结果写入 transcript 前运行。
type AfterToolCallFunc func(context.Context, AfterToolCallContext) (AfterToolCallResult, error)

type toolCallExecutionOptions struct {
	assistantMessage AssistantMessage
	context          AgentContext
	validator        ArgumentValidator
	before           BeforeToolCallFunc
	after            AfterToolCallFunc
}
