package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestRunAgentLoop_injectsSteeringAfterCompleteToolBatch(t *testing.T) {
	queued := UserMessage{Content: []UserContent{TextContent{Text: "中途指令"}}}
	executed := make([]string, 0, 2)
	tools := []Tool{
		&recordingNamedTool{name: "first", executed: &executed},
		&recordingNamedTool{name: "second", executed: &executed},
	}
	modelCalls := 0
	polls := 0
	sawQueued := false
	var messageStarts []string

	messages, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: tools},
		LoopConfig{
			ToolExecution: ToolExecutionModeSequential,
			GetSteeringMessages: func(context.Context) ([]AgentMessage, error) {
				polls++
				if len(executed) == 2 && polls == 2 {
					return []AgentMessage{queued}, nil
				}
				return nil, nil
			},
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
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
				sawQueued = containsUserText(modelContext.Messages, "中途指令")
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
		},
		func(event AgentEvent) error {
			start, ok := event.(AgentMessageStartEvent)
			if !ok {
				return nil
			}
			switch message := start.Message.(type) {
			case ToolResultMessage:
				messageStarts = append(messageStarts, "tool:"+message.ToolCallID)
			case UserMessage:
				if containsUserText([]AgentMessage{message}, "中途指令") {
					messageStarts = append(messageStarts, "queued")
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(executed, []string{"first", "second"}) || !sawQueued {
		t.Fatalf("executed = %#v, saw queued = %v", executed, sawQueued)
	}
	if !reflect.DeepEqual(messageStarts, []string{"tool:call-1", "tool:call-2", "queued"}) {
		t.Fatalf("message starts = %#v, want tool results before queued", messageStarts)
	}
	if modelCalls != 2 || polls != 3 || len(messages) != 5 {
		t.Fatalf("calls/messages = model:%d polls:%d messages:%d, want 2/3/5", modelCalls, polls, len(messages))
	}
}

func TestRunAgentLoop_processesFollowUpOnlyWhenOtherwiseStopped(t *testing.T) {
	followUp := UserMessage{Content: []UserContent{TextContent{Text: "追问"}}}
	modelCalls := 0
	polls := 0
	secondTurnSawFollowUp := false

	messages, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				modelCalls++
				if modelCalls == 2 {
					secondTurnSawFollowUp = containsUserText(modelContext.Messages, "追问")
				}
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			GetFollowUpMessages: func(context.Context) ([]AgentMessage, error) {
				polls++
				if polls == 1 {
					return []AgentMessage{followUp}, nil
				}
				return nil, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 2 || polls != 2 || !secondTurnSawFollowUp || len(messages) != 3 {
		t.Fatalf("calls/state = model:%d polls:%d saw:%v messages:%d, want 2/2/true/3", modelCalls, polls, secondTurnSawFollowUp, len(messages))
	}
}

type recordingNamedTool struct {
	name     string
	executed *[]string
}

func (tool *recordingNamedTool) Definition() ToolDefinition {
	return ToolDefinition{Name: tool.name}
}

func (tool *recordingNamedTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	*tool.executed = append(*tool.executed, tool.name)
	return ToolResult{}, nil
}

func containsUserText(messages []AgentMessage, want string) bool {
	for _, candidate := range messages {
		message, ok := candidate.(UserMessage)
		if !ok {
			continue
		}
		for _, candidateContent := range message.Content {
			content, ok := candidateContent.(TextContent)
			if ok && content.Text == want {
				return true
			}
		}
	}
	return false
}
