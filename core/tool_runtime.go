package agent

import (
	"context"
	"fmt"
	"sync"
)

// findToolByName 根据模型返回的工具名称查找可用工具。
func findToolByName(tools []Tool, name string) (Tool, bool) {
	for _, tool := range tools {
		if tool == nil {
			continue
		}

		if tool.Definition().Name == name {
			return tool, true
		}
	}

	return nil, false
}

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
	// 工具执行采用固定管线：按名称查找 -> 参数规范化 -> Schema 校验 -> Before Hook。
	// 任一步失败都不会调用 Tool.Execute，而是生成普通 ToolResultMessage 反馈给 Model，
	// 让 Model 有机会修正工具名或参数，而不是直接打断整个 Agent Loop。
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

// executeToolCall 执行一次工具调用，并将结果转换为标准消息。
func executeToolCall(
	ctx context.Context,
	call ToolCall,
	timestamp int64,
	onUpdate toolUpdateSink,
	options toolCallExecutionOptions,
) (toolCallOutcome, error) {
	preparation := prepareToolCall(ctx, call, timestamp, options)
	if !preparation.execute {
		return preparation.immediate, nil
	}
	return executePreparedToolCall(ctx, preparation, onUpdate)
}

func executePreparedToolCall(
	ctx context.Context,
	preparation toolCallPreparation,
	onUpdate toolUpdateSink,
) (toolCallOutcome, error) {
	// ToolUpdateFunc 只在 Execute 返回前有效，避免工具内部残留 goroutine 在下一轮继续
	// 向旧调用发送 partial result。最终结果随后经过 After Hook 再写入 transcript。
	updates := newToolUpdateGate(onUpdate)
	result, err := preparation.tool.Execute(
		ctx,
		preparation.preparedCall,
		updates.update,
	)
	if updateError := updates.settle(); updateError != nil {
		return toolCallOutcome{}, updateError
	}
	isError := false
	if err != nil {
		result = newErrorToolResult(err.Error())
		isError = true
	}
	result, isError = runAfterToolCall(
		ctx,
		preparation.options,
		preparation.call,
		preparation.preparedCall,
		result,
		isError,
	)
	return newToolCallOutcome(
		preparation.call,
		result,
		isError,
		preparation.timestamp,
	), nil
}

type toolCallOutcome struct {
	message   ToolResultMessage
	result    ToolResult
	isError   bool
	terminate bool
}

func newToolCallOutcome(
	call ToolCall,
	result ToolResult,
	isError bool,
	timestamp int64,
) toolCallOutcome {
	// 同时保留 ToolResult（事件需要）和 ToolResultMessage（对话历史需要），避免协议层
	// 与执行层相互依赖。
	return toolCallOutcome{
		message:   newToolResultMessage(call, result, isError, timestamp),
		result:    result,
		isError:   isError,
		terminate: result.Terminate,
	}
}

// newErrorToolResult 创建可返回给模型的错误工具结果。
func newErrorToolResult(message string) ToolResult {
	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: message},
		},
		Details: map[string]any{},
	}
}

// newToolResultMessage 将工具执行结果转换为对话消息。
func newToolResultMessage(
	call ToolCall,
	result ToolResult,
	isError bool,
	timestamp int64,
) ToolResultMessage {
	content := result.Content
	if content == nil {
		content = []ToolResultContent{}
	}

	return ToolResultMessage{
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		Content:        content,
		Details:        result.Details,
		Usage:          result.Usage,
		AddedToolNames: result.AddedToolNames,
		IsError:        isError,
		Timestamp:      timestamp,
	}
}

type toolUpdateSink func(ToolResult) error

type toolUpdateGate struct {
	mutex     sync.Mutex
	accepting bool
	sink      toolUpdateSink
	err       error
}

func newToolUpdateGate(sink toolUpdateSink) *toolUpdateGate {
	return &toolUpdateGate{accepting: true, sink: sink}
}

func (gate *toolUpdateGate) update(partial ToolResult) {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if !gate.accepting || gate.err != nil || gate.sink == nil {
		return
	}
	gate.err = gate.sink(partial)
}

func (gate *toolUpdateGate) settle() error {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	gate.accepting = false
	return gate.err
}
