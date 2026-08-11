package agent

import (
	"context"
	"fmt"
)

func runBeforeToolCall(
	ctx context.Context,
	hook BeforeToolCallFunc,
	options toolCallExecutionOptions,
	rawCall ToolCall,
	preparedCall *ToolCall,
) (BeforeToolCallResult, error) {
	if hook == nil {
		return BeforeToolCallResult{}, nil
	}
	hookContext, err := newBeforeToolCallContext(options, rawCall, *preparedCall)
	if err != nil {
		return BeforeToolCallResult{}, err
	}
	result, err := hook(ctx, hookContext)
	preparedCall.Arguments = hookContext.Arguments
	return result, err
}

func newBeforeToolCallContext(
	options toolCallExecutionOptions,
	rawCall ToolCall,
	preparedCall ToolCall,
) (BeforeToolCallContext, error) {
	cloner := newSnapshotCloner()
	assistant, err := cloner.cloneAssistantMessage(options.assistantMessage)
	if err != nil {
		return BeforeToolCallContext{}, err
	}
	call, err := cloner.cloneToolCall(rawCall)
	if err != nil {
		return BeforeToolCallContext{}, err
	}
	current, err := cloneAgentContext(options.context)
	if err != nil {
		return BeforeToolCallContext{}, err
	}
	arguments, err := cloneArguments(preparedCall.Arguments)
	if err != nil {
		return BeforeToolCallContext{}, err
	}
	return BeforeToolCallContext{
		AssistantMessage: assistant, ToolCall: call,
		Arguments: arguments, Context: current,
	}, nil
}

func runAfterToolCall(
	ctx context.Context,
	options toolCallExecutionOptions,
	rawCall ToolCall,
	preparedCall ToolCall,
	result ToolResult,
	isError bool,
) (ToolResult, bool) {
	if options.after == nil {
		return result, isError
	}
	hookContext, err := newAfterToolCallContext(
		options, rawCall, preparedCall, result, isError,
	)
	if err != nil {
		return newErrorToolResult(fmt.Sprintf("Tool %q after hook snapshot failed: %v", rawCall.Name, err)), true
	}
	override, err := options.after(ctx, hookContext)
	if err != nil {
		return newErrorToolResult(fmt.Sprintf("Tool %q after hook failed: %v", rawCall.Name, err)), true
	}
	result, isError = applyAfterToolCallResult(result, isError, override)
	cloned, err := newSnapshotCloner().cloneToolResult(result)
	if err != nil {
		return newErrorToolResult(fmt.Sprintf("Tool %q after hook result failed: %v", rawCall.Name, err)), true
	}
	return cloned, isError
}

func newAfterToolCallContext(
	options toolCallExecutionOptions,
	rawCall ToolCall,
	preparedCall ToolCall,
	result ToolResult,
	isError bool,
) (AfterToolCallContext, error) {
	before, err := newBeforeToolCallContext(options, rawCall, preparedCall)
	if err != nil {
		return AfterToolCallContext{}, err
	}
	resultSnapshot, err := newSnapshotCloner().cloneToolResult(result)
	if err != nil {
		return AfterToolCallContext{}, err
	}
	return AfterToolCallContext{
		AssistantMessage: before.AssistantMessage,
		ToolCall:         before.ToolCall,
		Arguments:        before.Arguments,
		Result:           resultSnapshot,
		IsError:          isError,
		Context:          before.Context,
	}, nil
}

func applyAfterToolCallResult(
	result ToolResult,
	isError bool,
	override AfterToolCallResult,
) (ToolResult, bool) {
	if override.Content != nil {
		result.Content = override.Content
	}
	if override.Details != nil {
		result.Details = override.Details
	}
	if override.Usage != nil {
		result.Usage = override.Usage
	}
	if override.Terminate != nil {
		result.Terminate = *override.Terminate
	}
	if override.IsError != nil {
		isError = *override.IsError
	}
	return result, isError
}
