package agent

import "context"

// ArgumentValidator 校验一次工具调用的最终参数。
type ArgumentValidator interface {
	Validate(context.Context, ToolDefinition, map[string]any) error
}

// ArgumentValidatorFunc 将函数适配为 ArgumentValidator。
type ArgumentValidatorFunc func(context.Context, ToolDefinition, map[string]any) error

func (validator ArgumentValidatorFunc) Validate(
	ctx context.Context,
	definition ToolDefinition,
	arguments map[string]any,
) error {
	return validator(ctx, definition, arguments)
}

// ToolArgumentPreparer 是 Tool 可选实现的参数规范化接口。
type ToolArgumentPreparer interface {
	PrepareArguments(map[string]any) (map[string]any, error)
}

func prepareToolCallArguments(tool Tool, call ToolCall) (ToolCall, error) {
	arguments, err := cloneArguments(call.Arguments)
	if err != nil {
		return ToolCall{}, err
	}
	call.Arguments = arguments
	preparer, ok := tool.(ToolArgumentPreparer)
	if !ok {
		return call, nil
	}
	prepared, err := preparer.PrepareArguments(arguments)
	if err != nil {
		return ToolCall{}, err
	}
	call.Arguments = prepared
	return call, nil
}
