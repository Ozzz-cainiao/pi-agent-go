package agent

import (
	"context"
	"encoding/json"
)

// ToolExecutionMode 表示工具调用的执行方式。
type ToolExecutionMode string

const (
	// ToolExecutionModeSequential 表示工具调用按顺序执行。
	ToolExecutionModeSequential ToolExecutionMode = "sequential"

	// ToolExecutionModeParallel 表示工具调用可以并行执行。
	ToolExecutionModeParallel ToolExecutionMode = "parallel"
)

// ToolDefinition 表示提供给模型的工具定义。
type ToolDefinition struct {
	Name          string
	Label         string
	Description   string
	Parameters    json.RawMessage
	ExecutionMode ToolExecutionMode
}

// ToolResult 表示工具执行产生的最终或中间结果。
type ToolResult struct {
	Content        []ToolResultContent
	Details        any
	Usage          *Usage
	AddedToolNames []string
	Terminate      bool
}

// ToolUpdateFunc 接收工具执行期间产生的中间结果。
type ToolUpdateFunc func(ToolResult)

// Tool 表示 Agent Loop 可以调用的工具。
type Tool interface {
	Definition() ToolDefinition

	Execute(
		ctx context.Context,
		call ToolCall,
		onUpdate ToolUpdateFunc,
	) (ToolResult, error)
}
