package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCLI_liveProvider(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv(credentialEnvironment))
	model := strings.TrimSpace(os.Getenv(modelEnvironment))
	if apiKey == "" || model == "" {
		t.Skip("live test skipped: OPENAI_API_KEY and OPENAI_RESPONSES_MODEL are required")
	}

	// Given
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var output bytes.Buffer

	// When
	err := run(ctx, []string{"-prompt", "请直接回复 OK，不要调用工具。"}, cliDependencies{
		getenv: os.Getenv,
		stdout: &output,
	})
	// Then
	if err != nil {
		t.Fatalf("live run returned error: %v", err)
	}
	if strings.TrimSpace(output.String()) == "" {
		t.Fatal("live run returned empty Assistant text")
	}
	if strings.Contains(output.String(), apiKey) {
		t.Fatal("live output exposed API key")
	}
}
