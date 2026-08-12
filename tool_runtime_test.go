package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubTool struct{}

func (stubTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "qa_search",
		Label:       "QA Search",
		Description: "搜索知识库",
	}
}

func (stubTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: "搜索结果"},
		},
	}, nil
}

func TestTool_contract(t *testing.T) {
	// 准备
	var tool Tool = stubTool{}

	// 执行
	definition := tool.Definition()
	result, err := tool.Execute(
		context.Background(),
		ToolCall{
			ID:   "call-1",
			Name: "qa_search",
		},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if definition.Name != "qa_search" {
		t.Fatalf("Definition().Name = %q, want %q", definition.Name, "qa_search")
	}
	if len(result.Content) != 1 {
		t.Fatalf("result content count = %d, want 1", len(result.Content))
	}
}

func TestFindToolByName_returnsMatchingTool(t *testing.T) {
	// 准备
	tools := []Tool{
		nil,
		stubTool{},
	}

	// 执行
	got, ok := findToolByName(tools, "qa_search")

	// 验证
	if !ok {
		t.Fatal("findToolByName() did not find qa_search")
	}
	if got.Definition().Name != "qa_search" {
		t.Fatalf(
			"tool name = %q, want %q",
			got.Definition().Name,
			"qa_search",
		)
	}
}

func TestFindToolByName_returnsFalseForMissingTool(t *testing.T) {
	// 准备
	tools := []Tool{
		stubTool{},
	}

	// 执行
	got, ok := findToolByName(tools, "rag_search")

	// 验证
	if ok {
		t.Fatalf("findToolByName() found unexpected tool: %#v", got)
	}
	if got != nil {
		t.Fatalf("findToolByName() tool = %#v, want nil", got)
	}
}

func TestExecuteToolCall_returnsSuccessfulResultMessage(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-1",
		Name: "qa_search",
		Arguments: map[string]any{
			"query": "什么是 Agent Loop？",
		},
	}

	// 执行
	outcome, err := executeToolCall(
		context.Background(),
		call,
		100,
		func(ToolResult) error { return nil },
		toolCallExecutionOptions{
			context:   AgentContext{Tools: []Tool{stubTool{}}},
			validator: JSONSchemaArgumentValidator{},
		},
	)
	if err != nil {
		t.Fatalf("executeToolCall() returned error: %v", err)
	}
	got := outcome.message

	// 验证
	want := ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "qa_search",
		Content: []ToolResultContent{
			TextContent{Text: "搜索结果"},
		},
		IsError:   false,
		Timestamp: 100,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("executeToolCall() = %#v, want %#v", got, want)
	}
}

func TestExecuteToolCall_returnsErrorForMissingTool(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-2",
		Name: "rag_search",
	}

	// 执行
	outcome, err := executeToolCall(
		context.Background(),
		call,
		200,
		func(ToolResult) error { return nil },
		toolCallExecutionOptions{
			context:   AgentContext{Tools: []Tool{stubTool{}}},
			validator: JSONSchemaArgumentValidator{},
		},
	)
	if err != nil {
		t.Fatalf("executeToolCall() returned error: %v", err)
	}
	got := outcome.message

	// 验证
	if !got.IsError {
		t.Fatal("executeToolCall() IsError = false, want true")
	}
	if got.ToolCallID != "call-2" {
		t.Fatalf("ToolCallID = %q, want %q", got.ToolCallID, "call-2")
	}

	wantContent := []ToolResultContent{
		TextContent{Text: "Tool rag_search not found"},
	}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", got.Content, wantContent)
	}
}

var errToolBackend = errors.New("tool backend unavailable")

type failingTool struct{}

func (failingTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name: "failing_tool",
	}
}

func (failingTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	return ToolResult{}, errToolBackend
}

func TestExecuteToolCall_convertsToolErrorToResultMessage(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-3",
		Name: "failing_tool",
	}

	// 执行
	outcome, err := executeToolCall(
		context.Background(),
		call,
		300,
		func(ToolResult) error { return nil },
		toolCallExecutionOptions{
			context:   AgentContext{Tools: []Tool{failingTool{}}},
			validator: JSONSchemaArgumentValidator{},
		},
	)
	if err != nil {
		t.Fatalf("executeToolCall() returned error: %v", err)
	}
	got := outcome.message

	// 验证
	if !got.IsError {
		t.Fatal("executeToolCall() IsError = false, want true")
	}

	wantContent := []ToolResultContent{
		TextContent{Text: errToolBackend.Error()},
	}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", got.Content, wantContent)
	}
}

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

type terminatingTool struct {
	name      string
	terminate bool
}

func (tool terminatingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool terminatingTool) Execute(
	context.Context,
	ToolCall,
	ToolUpdateFunc,
) (ToolResult, error) {
	return ToolResult{
		Content:   []ToolResultContent{TextContent{Text: tool.name + " done"}},
		Terminate: tool.terminate,
	}, nil
}

func TestRunAgentLoop_stopsOnlyWhenEveryToolResultTerminates(t *testing.T) {
	tests := []struct {
		name           string
		terminates     []bool
		afterTerminate *bool
		wantModelCalls int
	}{
		{name: "all terminate", terminates: []bool{true, true}, wantModelCalls: 1},
		{name: "mixed batch", terminates: []bool{true, false}, wantModelCalls: 2},
		{name: "none terminate", terminates: []bool{false, false}, wantModelCalls: 2},
		{
			name: "after hook finalizes all as terminate", terminates: []bool{false, false},
			afterTerminate: boolPointer(true), wantModelCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelCalls := 0
			messages, err := RunAgentLoop(
				context.Background(),
				nil,
				AgentContext{Tools: []Tool{
					terminatingTool{name: "first", terminate: test.terminates[0]},
					terminatingTool{name: "second", terminate: test.terminates[1]},
				}},
				LoopConfig{
					AfterToolCall: func(context.Context, AfterToolCallContext) (AfterToolCallResult, error) {
						return AfterToolCallResult{Terminate: test.afterTerminate}, nil
					},
					Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
						modelCalls++
						if modelCalls == 1 {
							return AssistantMessage{
								Content: []AssistantContent{
									ToolCall{ID: "call-1", Name: "first"},
									ToolCall{ID: "call-2", Name: "second"},
								},
								StopReason: StopReasonToolUse,
							}, nil
						}
						return AssistantMessage{StopReason: StopReasonStop}, nil
					},
				},
				nil,
			)
			if err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
			if modelCalls != test.wantModelCalls {
				t.Fatalf("Model calls = %d, want %d", modelCalls, test.wantModelCalls)
			}
			wantMessages := 3
			if test.wantModelCalls == 2 {
				wantMessages = 4
			}
			if len(messages) != wantMessages {
				t.Fatalf("new messages = %d, want %d", len(messages), wantMessages)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

type lateUpdateTool struct {
	callback chan<- ToolUpdateFunc
}

func (lateUpdateTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "first"}
}

func (tool lateUpdateTool) Execute(
	_ context.Context,
	_ ToolCall,
	onUpdate ToolUpdateFunc,
) (ToolResult, error) {
	onUpdate(ToolResult{Content: []ToolResultContent{TextContent{Text: "during"}}})
	tool.callback <- onUpdate
	return ToolResult{Content: []ToolResultContent{TextContent{Text: "first done"}}}, nil
}

type barrierTool struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (barrierTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "second"}
}

func (tool barrierTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	close(tool.started)
	<-tool.release
	return ToolResult{Content: []ToolResultContent{TextContent{Text: "second done"}}}, nil
}

func TestRunAgentLoop_discardsLateToolUpdateWhileAnotherToolRuns(t *testing.T) {
	callback := make(chan ToolUpdateFunc, 1)
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	var updateMutex sync.Mutex
	var updates []string
	finished := make(chan error, 1)
	modelCall := 0

	go func() {
		_, err := RunAgentLoop(
			context.Background(),
			nil,
			AgentContext{Tools: []Tool{
				lateUpdateTool{callback: callback},
				barrierTool{started: secondStarted, release: releaseSecond},
			}},
			LoopConfig{Stream: func(context.Context, AgentContext, AssistantMessageEventSink) (AssistantMessage, error) {
				modelCall++
				if modelCall == 1 {
					return AssistantMessage{
						Content: []AssistantContent{
							ToolCall{ID: "call-1", Name: "first"},
							ToolCall{ID: "call-2", Name: "second"},
						},
						StopReason: StopReasonToolUse,
					}, nil
				}
				return AssistantMessage{StopReason: StopReasonStop}, nil
			}},
			func(event AgentEvent) error {
				update, ok := event.(AgentToolExecutionUpdateEvent)
				if !ok {
					return nil
				}
				updateMutex.Lock()
				updates = append(updates, toolResultContentText(t, update.PartialResult))
				updateMutex.Unlock()
				return nil
			},
		)
		finished <- err
	}()

	var lateCallback ToolUpdateFunc
	select {
	case lateCallback = <-callback:
	case <-time.After(2 * time.Second):
		t.Fatal("first tool did not expose update callback")
	}
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second tool did not start")
	}
	lateCallback(ToolResult{Content: []ToolResultContent{TextContent{Text: "late"}}})
	close(releaseSecond)
	if err := <-finished; err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}

	updateMutex.Lock()
	defer updateMutex.Unlock()
	if len(updates) != 1 || updates[0] != "during" {
		t.Fatalf("updates = %#v, want only settle-before update", updates)
	}
}

func toolResultContentText(t *testing.T, result ToolResult) string {
	t.Helper()
	text, ok := result.Content[0].(TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T, want TextContent", result.Content[0])
	}
	return text.Text
}
