package agent

import (
	"reflect"
	"testing"
)

func TestAssistantMessageEventSink_forwardsEveryEventVariant(t *testing.T) {
	partial := AssistantMessage{StopReason: StopReasonPending}
	final := AssistantMessage{StopReason: StopReasonStop}
	toolCall := ToolCall{ID: "call-1", Name: "search"}
	want := []AssistantMessageEvent{
		AssistantStartEvent{Partial: partial},
		AssistantTextStartEvent{ContentIndex: 0, Partial: partial},
		AssistantTextDeltaEvent{ContentIndex: 0, Delta: "你", Partial: partial},
		AssistantTextEndEvent{ContentIndex: 0, Content: "你好", Partial: partial},
		AssistantThinkingStartEvent{ContentIndex: 1, Partial: partial},
		AssistantThinkingDeltaEvent{ContentIndex: 1, Delta: "分析", Partial: partial},
		AssistantThinkingEndEvent{ContentIndex: 1, Content: "分析完成", Partial: partial},
		AssistantToolCallStartEvent{ContentIndex: 2, Partial: partial},
		AssistantToolCallDeltaEvent{ContentIndex: 2, Delta: `{"query":"pi"`, Partial: partial},
		AssistantToolCallEndEvent{ContentIndex: 2, ToolCall: toolCall, Partial: partial},
		AssistantDoneEvent{Reason: StopReasonStop, Message: final},
		AssistantErrorEvent{
			Reason: StopReasonError,
			Error: AssistantMessage{
				StopReason:   StopReasonError,
				ErrorMessage: "provider failed",
			},
		},
	}

	got := make([]AssistantMessageEvent, 0, len(want))
	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		got = append(got, event)

		return nil
	})
	for _, event := range want {
		if err := emit(event); err != nil {
			t.Fatalf("emit(%T) returned error: %v", event, err)
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}
