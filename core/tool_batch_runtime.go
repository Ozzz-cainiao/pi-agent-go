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
	// 并行是整批决定：只要全局配置或任一工具要求串行，整批就按源顺序执行。
	// 这样具有副作用的工具可以通过 Definition.ExecutionMode 明确禁止并行。
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

type parallelToolEntry struct {
	preparation toolCallPreparation
	outcome     toolCallOutcome
	execute     bool
}

type indexedToolOutcome struct {
	index   int
	outcome toolCallOutcome
	err     error
}

func executeToolCallsParallel(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	response AssistantMessage,
	calls []ToolCall,
) ([]ToolResultMessage, bool, error) {
	entries, err := prepareParallelToolCalls(
		ctx, runtime, events, state, response, calls,
	)
	if err != nil {
		return nil, false, err
	}
	if err := executeParallelToolCalls(ctx, events, entries); err != nil {
		return nil, false, err
	}
	return emitParallelToolMessages(events, state, entries)
}

func prepareParallelToolCalls(
	ctx context.Context,
	runtime loopRuntime,
	events agentEventEmitter,
	state *loopState,
	response AssistantMessage,
	calls []ToolCall,
) ([]parallelToolEntry, error) {
	// 第一阶段仍按模型给出的顺序完成 start、prepare、validate 和 Before Hook。
	// 只有真正的 Tool.Execute 进入并行阶段，因此前置策略的事件顺序是确定的。
	entries := make([]parallelToolEntry, len(calls))
	for index, call := range calls {
		if err := events.emit(AgentToolExecutionStartEvent{
			ToolCallID: call.ID, ToolName: call.Name, Arguments: call.Arguments,
		}); err != nil {
			return nil, err
		}
		preparation := prepareToolCall(
			ctx,
			call,
			runtime.clock().UnixMilli(),
			newToolCallExecutionOptions(runtime, state.current, response),
		)
		entries[index] = parallelToolEntry{
			preparation: preparation,
			outcome:     preparation.immediate,
			execute:     preparation.execute,
		}
		if !preparation.execute {
			if err := events.emit(toolExecutionEndEvent(preparation.immediate)); err != nil {
				return nil, err
			}
		}
	}
	return entries, nil
}

func executeParallelToolCalls(
	ctx context.Context,
	events agentEventEmitter,
	entries []parallelToolEntry,
) error {
	// worker 可以按任意顺序完成，但结果写回 entries 的原索引。于是执行结束事件反映
	// 真实完成顺序，而发回 Model 的 ToolResultMessage 仍保持 ToolCall 源顺序。
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	workerCount := 0
	results := make(chan indexedToolOutcome, len(entries))
	for index := range entries {
		if !entries[index].execute {
			continue
		}
		workerCount++
		go runParallelTool(workerContext, events, index, entries[index].preparation, results)
	}
	var firstError error
	for range workerCount {
		result := <-results
		entries[result.index].outcome = result.outcome
		if result.err != nil && firstError == nil {
			firstError = result.err
			cancel()
		}
	}
	return firstError
}

func runParallelTool(
	ctx context.Context,
	events agentEventEmitter,
	index int,
	preparation toolCallPreparation,
	results chan<- indexedToolOutcome,
) {
	call := preparation.call
	onUpdate := func(partial ToolResult) error {
		return events.emit(AgentToolExecutionUpdateEvent{
			ToolCallID: call.ID, ToolName: call.Name,
			Arguments: call.Arguments, PartialResult: partial,
		})
	}
	outcome, err := executePreparedToolCall(ctx, preparation, onUpdate)
	if err == nil {
		err = events.emit(toolExecutionEndEvent(outcome))
	}
	results <- indexedToolOutcome{index: index, outcome: outcome, err: err}
}

func emitParallelToolMessages(
	events agentEventEmitter,
	state *loopState,
	entries []parallelToolEntry,
) ([]ToolResultMessage, bool, error) {
	results := make([]ToolResultMessage, 0, len(entries))
	allTerminate := len(entries) > 0
	for _, entry := range entries {
		message := entry.outcome.message
		appendToolResult(state, message)
		if err := emitMessageLifecycle(events, message); err != nil {
			return results, false, err
		}
		results = append(results, message)
		allTerminate = allTerminate && entry.outcome.terminate
	}
	return results, allTerminate, nil
}

func newToolCallExecutionOptions(
	runtime loopRuntime,
	current AgentContext,
	response AssistantMessage,
) toolCallExecutionOptions {
	return toolCallExecutionOptions{
		assistantMessage: response,
		context:          current,
		validator:        runtime.argumentValidator,
		before:           runtime.beforeToolCall,
		after:            runtime.afterToolCall,
	}
}
