package agent

import (
	"context"
	"testing"
)

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
