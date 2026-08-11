package agent

import (
	"context"
	"fmt"
)

type toolCallPreparation struct {
	call         ToolCall
	preparedCall ToolCall
	tool         Tool
	timestamp    int64
	options      toolCallExecutionOptions
	execute      bool
	immediate    toolCallOutcome
}

func prepareToolCall(
	ctx context.Context,
	call ToolCall,
	timestamp int64,
	options toolCallExecutionOptions,
) toolCallPreparation {
	tool, ok := findToolByName(options.context.Tools, call.Name)
	if !ok {
		return immediateToolCall(
			call,
			newErrorToolResult("Tool "+call.Name+" not found"),
			timestamp,
		)
	}
	preparedCall, err := prepareToolCallArguments(tool, call)
	if err != nil {
		return immediateToolCall(call, newErrorToolResult(fmt.Sprintf(
			"Tool %q argument preparation failed: %v", call.Name, err,
		)), timestamp)
	}
	if err := options.validator.Validate(ctx, tool.Definition(), preparedCall.Arguments); err != nil {
		return immediateToolCall(call, newErrorToolResult(fmt.Sprintf(
			"Tool %q arguments invalid: %v", call.Name, err,
		)), timestamp)
	}
	beforeResult, err := runBeforeToolCall(
		ctx, options.before, options, call, &preparedCall,
	)
	if err != nil {
		return immediateToolCall(call, newErrorToolResult(fmt.Sprintf(
			"Tool %q before hook failed: %v", call.Name, err,
		)), timestamp)
	}
	if beforeResult.Block {
		reason := beforeResult.Reason
		if reason == "" {
			reason = "Tool execution was blocked"
		}
		result := newErrorToolResult(reason)
		result.Terminate = beforeResult.Terminate
		return immediateToolCall(call, result, timestamp)
	}
	return toolCallPreparation{
		call: call, preparedCall: preparedCall, tool: tool,
		timestamp: timestamp, options: options, execute: true,
	}
}

func immediateToolCall(
	call ToolCall,
	result ToolResult,
	timestamp int64,
) toolCallPreparation {
	return toolCallPreparation{
		call:      call,
		immediate: newToolCallOutcome(call, result, true, timestamp),
	}
}
