package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

var countSchema = json.RawMessage(`{
  "type": "object",
  "properties": {"count": {"type": "integer"}},
  "required": ["count"],
  "additionalProperties": false
}`)

type validationTool struct {
	executeCalls *int
	gotCall      *ToolCall
	order        *[]string
}

func (tool validationTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "counter", Parameters: countSchema}
}

func (tool validationTool) Execute(
	_ context.Context,
	call ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	*tool.executeCalls++
	*tool.gotCall = call
	if tool.order != nil {
		*tool.order = append(*tool.order, "execute")
	}
	return ToolResult{Content: []ToolResultContent{TextContent{Text: "ok"}}}, nil
}

type preparingValidationTool struct{ validationTool }

func (tool preparingValidationTool) PrepareArguments(arguments map[string]any) (map[string]any, error) {
	if tool.order != nil {
		*tool.order = append(*tool.order, "prepare")
	}
	prepared := make(map[string]any, len(arguments))
	for key, value := range arguments {
		prepared[key] = value
	}
	text, ok := prepared["count"].(string)
	if !ok {
		return prepared, nil
	}
	count, err := strconv.Atoi(text)
	if err != nil {
		return nil, err
	}
	prepared["count"] = count
	return prepared, nil
}

func TestRunAgentLoop_validatesToolArgumentsBeforeExecute(t *testing.T) {
	executeCalls := 0
	gotCall := ToolCall{}
	tool := validationTool{executeCalls: &executeCalls, gotCall: &gotCall}
	result := runToolArgumentScenario(t, tool, map[string]any{"count": 3}, nil)

	if result.IsError {
		t.Fatalf("tool result = %#v, want success", result)
	}
	if executeCalls != 1 || !reflect.DeepEqual(gotCall.Arguments, map[string]any{"count": 3}) {
		t.Fatalf("Execute calls/args = %d/%#v, want 1/valid args", executeCalls, gotCall.Arguments)
	}
}

func TestRunAgentLoop_preparesArgumentsBeforeValidation(t *testing.T) {
	executeCalls := 0
	gotCall := ToolCall{}
	var order []string
	tool := preparingValidationTool{validationTool{
		executeCalls: &executeCalls, gotCall: &gotCall, order: &order,
	}}
	result := runToolArgumentScenario(t, tool, map[string]any{"count": "3"}, nil)

	if result.IsError {
		t.Fatalf("tool result = %#v, want success", result)
	}
	if !reflect.DeepEqual(order, []string{"prepare", "execute"}) {
		t.Fatalf("order = %#v, want prepare before execute", order)
	}
	if !reflect.DeepEqual(gotCall.Arguments, map[string]any{"count": 3}) {
		t.Fatalf("Execute args = %#v, want prepared integer", gotCall.Arguments)
	}
}

func TestRunAgentLoop_usesInjectedArgumentValidatorAfterPrepare(t *testing.T) {
	executeCalls := 0
	gotCall := ToolCall{}
	var order []string
	tool := preparingValidationTool{validationTool{
		executeCalls: &executeCalls, gotCall: &gotCall, order: &order,
	}}
	validatorError := errors.New("custom validation failed")
	validator := ArgumentValidatorFunc(func(
		_ context.Context,
		definition ToolDefinition,
		arguments map[string]any,
	) error {
		order = append(order, "validate")
		if definition.Name != "counter" {
			t.Fatalf("validator tool = %q, want counter", definition.Name)
		}
		if _, ok := arguments["count"].(int); !ok {
			t.Fatalf("validator args = %#v, want prepared integer", arguments)
		}
		return validatorError
	})

	result := runToolArgumentScenario(t, tool, map[string]any{"count": "3"}, validator)

	if !result.IsError || executeCalls != 0 {
		t.Fatalf("result/error calls = %#v/%d, want custom validation error and 0", result, executeCalls)
	}
	if !reflect.DeepEqual(order, []string{"prepare", "validate"}) {
		t.Fatalf("order = %#v, want prepare then validate", order)
	}
	message := toolResultText(t, result)
	if !strings.Contains(message, "counter") || !strings.Contains(message, validatorError.Error()) {
		t.Fatalf("validation error = %q, want tool name and custom error", message)
	}
}

func TestRunAgentLoop_returnsValidationErrorWithoutExecute(t *testing.T) {
	tests := []struct {
		name       string
		arguments  map[string]any
		schemaPath string
	}{
		{name: "missing required", arguments: map[string]any{}, schemaPath: "/required"},
		{name: "wrong type", arguments: map[string]any{"count": "3"}, schemaPath: "/properties/count/type"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executeCalls := 0
			gotCall := ToolCall{}
			tool := validationTool{executeCalls: &executeCalls, gotCall: &gotCall}
			result := runToolArgumentScenario(t, tool, test.arguments, nil)

			if !result.IsError || executeCalls != 0 {
				t.Fatalf("result/error calls = %#v/%d, want validation error and 0", result, executeCalls)
			}
			message := toolResultText(t, result)
			if !strings.Contains(message, "counter") || !strings.Contains(message, test.schemaPath) {
				t.Fatalf("validation error = %q, want tool name and schema path %q", message, test.schemaPath)
			}
		})
	}
}

func runToolArgumentScenario(
	t *testing.T,
	tool Tool,
	arguments map[string]any,
	validator ArgumentValidator,
) ToolResultMessage {
	t.Helper()
	responses := []AssistantMessage{
		{Content: []AssistantContent{ToolCall{ID: "call-1", Name: "counter", Arguments: arguments}}, StopReason: StopReasonToolUse},
		{StopReason: StopReasonStop},
	}
	modelCall := 0
	var result ToolResultMessage
	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{
			ArgumentValidator: validator,
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				if modelCall == 1 {
					message := modelContext.Messages[len(modelContext.Messages)-1]
					var ok bool
					result, ok = message.(ToolResultMessage)
					if !ok {
						t.Fatalf("last Model message type = %T, want ToolResultMessage", message)
					}
				}
				response := responses[modelCall]
				modelCall++
				return response, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	return result
}

func toolResultText(t *testing.T, result ToolResultMessage) string {
	t.Helper()
	text, ok := result.Content[0].(TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T, want TextContent", result.Content[0])
	}
	return text.Text
}
