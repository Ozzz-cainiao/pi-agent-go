package openairesponses

import (
	"context"
	"errors"
	"testing"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func TestProviderStream_returnsTypedProtocolErrorForMalformedJSON(t *testing.T) {
	t.Parallel()

	// Given
	provider := newStreamTestProvider(t, "data: {bad json}\n\n", 3, nil)

	// When
	_, err := provider.Stream(context.Background(), agent.AgentContext{}, nil)

	// Then
	assertProtocolError(t, err)
}

func TestProviderStream_returnsTypedProtocolErrorWithoutTerminalEvent(t *testing.T) {
	t.Parallel()

	// Given
	stream := sseData(t, map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_1"}}) +
		"data: [DONE]\n\n"
	provider := newStreamTestProvider(t, stream, 5, nil)

	// When
	_, err := provider.Stream(context.Background(), agent.AgentContext{}, nil)

	// Then
	assertProtocolError(t, err)
}

func assertProtocolError(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("Stream() error = %v, want ErrProtocol", err)
	}
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("Stream() error type = %T, want *ProtocolError", err)
	}
}
