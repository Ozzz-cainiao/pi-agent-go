package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
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

// ArgumentValidator 校验一次工具调用的最终参数。
type ArgumentValidator interface {
	Validate(context.Context, ToolDefinition, map[string]any) error
}

// ArgumentValidatorFunc 将函数适配为 ArgumentValidator。
type ArgumentValidatorFunc func(context.Context, ToolDefinition, map[string]any) error

// Validate 调用被适配的参数校验函数。
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

const toolArgumentsSchemaURL = "tool-arguments-schema.json"

// JSONSchemaArgumentValidator 使用 JSON Schema Draft 2020-12 校验参数。
type JSONSchemaArgumentValidator struct{}

// Validate 按工具定义中的 JSON Schema 校验参数。
func (JSONSchemaArgumentValidator) Validate(
	ctx context.Context,
	definition ToolDefinition,
	arguments map[string]any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(definition.Parameters) == 0 {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	var document any
	if err := json.Unmarshal(definition.Parameters, &document); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	if err := compiler.AddResource(toolArgumentsSchemaURL, document); err != nil {
		return fmt.Errorf("load schema: %w", err)
	}
	schema, err := compiler.Compile(toolArgumentsSchemaURL)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := schema.Validate(arguments); err != nil {
		return fmt.Errorf("schema path %s: %w", schemaPath(err), err)
	}
	return nil
}

func schemaPath(err error) string {
	var validationError *jsonschema.ValidationError
	if !errors.As(err, &validationError) {
		return "/"
	}
	for _, cause := range validationError.Causes {
		if path := schemaPath(cause); path != "/" {
			return path
		}
	}
	if validationError.ErrorKind != nil {
		keywords := validationError.ErrorKind.KeywordPath()
		if len(keywords) > 0 {
			return joinSchemaPath(schemaFragment(validationError.SchemaURL), keywords)
		}
	}
	return "/"
}

func schemaFragment(schemaURL string) []string {
	parsed, err := url.Parse(schemaURL)
	if err != nil || parsed.Fragment == "" {
		return nil
	}
	return strings.Split(strings.Trim(parsed.Fragment, "/"), "/")
}

func joinSchemaPath(fragment, keywords []string) string {
	parts := append(append([]string(nil), fragment...), keywords...)
	return "/" + strings.Join(parts, "/")
}
