package agenttest

import (
	"context"
	"sync"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

// ToolHandler 实现 scripted Tool 的单次调用行为。
type ToolHandler func(context.Context, agent.ToolCall, agent.ToolUpdateFunc) (agent.ToolResult, error)

// ScriptedTool 记录调用，并将执行委托给可注入 handler。
type ScriptedTool struct {
	mu         sync.Mutex
	definition agent.ToolDefinition
	handler    ToolHandler
	calls      []agent.ToolCall
}

// NewScriptedTool 创建一个定义和行为均可控制的 Tool。
func NewScriptedTool(definition agent.ToolDefinition, handler ToolHandler) *ScriptedTool {
	return &ScriptedTool{definition: definition, handler: handler}
}

// Definition 返回 Tool 定义。
func (tool *ScriptedTool) Definition() agent.ToolDefinition {
	return tool.definition
}

// Execute 记录参数副本后执行 handler；nil handler 返回空结果。
func (tool *ScriptedTool) Execute(
	ctx context.Context,
	call agent.ToolCall,
	onUpdate agent.ToolUpdateFunc,
) (agent.ToolResult, error) {
	tool.mu.Lock()
	tool.calls = append(tool.calls, cloneToolCall(call))
	tool.mu.Unlock()
	if tool.handler == nil {
		return agent.ToolResult{}, nil
	}
	return tool.handler(ctx, call, onUpdate)
}

// Calls 返回不会修改内部记录的 ToolCall 副本。
func (tool *ScriptedTool) Calls() []agent.ToolCall {
	tool.mu.Lock()
	defer tool.mu.Unlock()
	calls := make([]agent.ToolCall, len(tool.calls))
	for index, call := range tool.calls {
		calls[index] = cloneToolCall(call)
	}
	return calls
}

func cloneToolCall(call agent.ToolCall) agent.ToolCall {
	call.Arguments = cloneMap(call.Arguments)
	return call
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneValue(item)
		}
		return cloned
	default:
		return typed
	}
}
