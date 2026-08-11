package agent

import (
	"errors"
	"testing"
)

func TestParseStopReason_accepts_known_reasons(t *testing.T) {
	// Given
	tests := []struct {
		raw  string
		want StopReason
	}{
		{raw: "pending", want: StopReasonPending},
		{raw: "stop", want: StopReasonStop},
		{raw: "length", want: StopReasonLength},
		{raw: "toolUse", want: StopReasonToolUse},
		{raw: "error", want: StopReasonError},
		{raw: "aborted", want: StopReasonAborted},
		{raw: "deferred", want: StopReasonDeferred},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			// When
			got, err := ParseStopReason(tt.raw)

			// Then
			if err != nil {
				t.Fatalf("ParseStopReason(%q) returned error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseStopReason(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseStopReason_rejects_unknown_reason(t *testing.T) {
	// Given
	raw := "banana"

	// When
	_, err := ParseStopReason(raw)

	// Then
	if !errors.Is(err, ErrInvalidStopReason) {
		t.Fatalf("ParseStopReason(%q) error = %v, want ErrInvalidStopReason", raw, err)
	}
}
