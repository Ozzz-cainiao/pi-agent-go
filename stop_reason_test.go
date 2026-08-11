package agent

import "testing"

func TestParseStopReason_accepts_known_reason(t *testing.T) {
	// Given
	raw := "toolUse"

	// When
	got, err := ParseStopReason(raw)

	// Then
	if err != nil {
		t.Fatalf("ParseStopReason() returned error: %v", err)
	}
	if got != StopReasonToolUse {
		t.Fatalf("ParseStopReason() = %q, want %q", got, StopReasonToolUse)
	}
}