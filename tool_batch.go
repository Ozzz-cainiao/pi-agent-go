package agent

import "context"

func executeToolCalls(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	response AssistantMessage,
	calls []ToolCall,
) ([]ToolResultMessage, bool, error) {
	if toolBatchAllowsParallel(runtime, state.current.Tools, calls) {
		return executeToolCallsParallel(ctx, runtime, events, state, response, calls)
	}
	return executeToolCallsSequential(ctx, runtime, events, state, response, calls)
}

func toolBatchAllowsParallel(
	runtime loopRuntime,
	tools []Tool,
	calls []ToolCall,
) bool {
	if runtime.toolExecution != ToolExecutionModeParallel {
		return false
	}
	for _, call := range calls {
		tool, ok := findToolByName(tools, call.Name)
		if !ok {
			continue
		}
		mode := tool.Definition().ExecutionMode
		if mode != "" && mode != ToolExecutionModeParallel {
			return false
		}
	}
	return true
}
