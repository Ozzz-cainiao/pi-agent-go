package agent

import "context"

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
