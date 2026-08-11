package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRunAgentLoop_runsToolHooksInOrderWithoutRevalidation(t *testing.T) {
	executeCalls := 0
	gotCall := ToolCall{}
	var order []string
	tool := preparingValidationTool{validationTool{
		executeCalls: &executeCalls, gotCall: &gotCall, order: &order,
	}}
	validator := ArgumentValidatorFunc(func(
		_ context.Context,
		_ ToolDefinition,
		arguments map[string]any,
	) error {
		order = append(order, "validate")
		if _, ok := arguments["count"].(int); !ok {
			t.Fatalf("validator args = %#v, want prepared integer", arguments)
		}
		return nil
	})
	before := BeforeToolCallFunc(func(
		_ context.Context,
		hookContext BeforeToolCallContext,
	) (BeforeToolCallResult, error) {
		order = append(order, "before")
		hookContext.Arguments["count"] = "changed-after-validation"
		return BeforeToolCallResult{}, nil
	})
	usage := Usage{InputTokens: 9, TotalTokens: 9}
	after := AfterToolCallFunc(func(
		_ context.Context,
		hookContext AfterToolCallContext,
	) (AfterToolCallResult, error) {
		order = append(order, "after")
		if hookContext.Arguments["count"] != "changed-after-validation" {
			t.Fatalf("after args = %#v, want before override", hookContext.Arguments)
		}
		return AfterToolCallResult{
			Content: []ToolResultContent{TextContent{Text: "overridden"}},
			Details: map[string]any{"source": "after"},
			Usage:   &usage,
		}, nil
	})

	result, _ := runToolHookScenario(t, tool, validator, before, after)

	wantOrder := []string{"prepare", "validate", "before", "execute", "after"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("order = %#v, want %#v", order, wantOrder)
	}
	if gotCall.Arguments["count"] != "changed-after-validation" {
		t.Fatalf("Execute args = %#v, want before override without revalidation", gotCall.Arguments)
	}
	if toolResultText(t, result) != "overridden" || !reflect.DeepEqual(result.Usage, &usage) {
		t.Fatalf("result/usage = %#v/%#v, want after override", result, result.Usage)
	}
	if !reflect.DeepEqual(result.Details, map[string]any{"source": "after"}) {
		t.Fatalf("details = %#v, want after override", result.Details)
	}
}

func TestRunAgentLoop_beforeToolHookCanBlockExecution(t *testing.T) {
	executeCalls := 0
	gotCall := ToolCall{}
	afterCalled := false
	tool := preparingValidationTool{validationTool{
		executeCalls: &executeCalls, gotCall: &gotCall,
	}}
	before := BeforeToolCallFunc(func(
		context.Context,
		BeforeToolCallContext,
	) (BeforeToolCallResult, error) {
		return BeforeToolCallResult{Block: true, Reason: "策略禁止"}, nil
	})
	after := AfterToolCallFunc(func(
		context.Context,
		AfterToolCallContext,
	) (AfterToolCallResult, error) {
		afterCalled = true
		return AfterToolCallResult{}, nil
	})

	result, kinds := runToolHookScenario(t, tool, nil, before, after)

	if !result.IsError || executeCalls != 0 || afterCalled {
		t.Fatalf("block result/calls = %#v/%d/%t, want error/0/false", result, executeCalls, afterCalled)
	}
	if !strings.Contains(toolResultText(t, result), "策略禁止") {
		t.Fatalf("block message = %q, want reason", toolResultText(t, result))
	}
	wantToolLifecycle := []AgentEventKind{
		AgentEventToolExecutionStart,
		AgentEventToolExecutionEnd,
	}
	if !reflect.DeepEqual(filterToolEvents(kinds), wantToolLifecycle) {
		t.Fatalf("tool lifecycle = %#v, want closed lifecycle", filterToolEvents(kinds))
	}
}

func TestRunAgentLoop_convertsToolHookErrorsAndClosesLifecycle(t *testing.T) {
	hookError := errors.New("hook failed")
	tests := []struct {
		name        string
		before      BeforeToolCallFunc
		after       AfterToolCallFunc
		wantExecute int
		wantStage   string
	}{
		{
			name: "before",
			before: func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error) {
				return BeforeToolCallResult{}, hookError
			},
			wantStage: "before",
		},
		{
			name: "after",
			after: func(context.Context, AfterToolCallContext) (AfterToolCallResult, error) {
				return AfterToolCallResult{}, hookError
			},
			wantExecute: 1,
			wantStage:   "after",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executeCalls := 0
			gotCall := ToolCall{}
			tool := preparingValidationTool{validationTool{
				executeCalls: &executeCalls, gotCall: &gotCall,
			}}
			result, kinds := runToolHookScenario(t, tool, nil, test.before, test.after)

			message := toolResultText(t, result)
			if !result.IsError || executeCalls != test.wantExecute {
				t.Fatalf("hook result/calls = %#v/%d", result, executeCalls)
			}
			if !strings.Contains(message, test.wantStage) || !strings.Contains(message, hookError.Error()) {
				t.Fatalf("hook error = %q, want stage and sentinel", message)
			}
			want := []AgentEventKind{AgentEventToolExecutionStart, AgentEventToolExecutionEnd}
			if !reflect.DeepEqual(filterToolEvents(kinds), want) {
				t.Fatalf("tool lifecycle = %#v, want closed lifecycle", filterToolEvents(kinds))
			}
		})
	}
}

func runToolHookScenario(
	t *testing.T,
	tool Tool,
	validator ArgumentValidator,
	before BeforeToolCallFunc,
	after AfterToolCallFunc,
) (ToolResultMessage, []AgentEventKind) {
	t.Helper()
	responses := []AssistantMessage{
		{Content: []AssistantContent{ToolCall{ID: "call-1", Name: "counter", Arguments: map[string]any{"count": "3"}}}, StopReason: StopReasonToolUse},
		{StopReason: StopReasonStop},
	}
	modelCall := 0
	var result ToolResultMessage
	var kinds []AgentEventKind
	_, err := RunAgentLoop(context.Background(), nil, AgentContext{Tools: []Tool{tool}}, LoopConfig{
		ArgumentValidator: validator,
		BeforeToolCall:    before,
		AfterToolCall:     after,
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
	}, func(event AgentEvent) error {
		kinds = append(kinds, event.Kind())
		return nil
	})
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	return result, kinds
}

func filterToolEvents(kinds []AgentEventKind) []AgentEventKind {
	var filtered []AgentEventKind
	for _, kind := range kinds {
		if kind == AgentEventToolExecutionStart || kind == AgentEventToolExecutionUpdate || kind == AgentEventToolExecutionEnd {
			filtered = append(filtered, kind)
		}
	}
	return filtered
}
