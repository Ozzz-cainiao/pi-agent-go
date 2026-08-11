package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_returnsSafeErrorWhenRequiredEnvironmentIsMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		env     map[string]string
		missing string
	}{
		{name: "api key", env: map[string]string{modelEnvironment: "gpt-test"}, missing: credentialEnvironment},
		{name: "model", env: map[string]string{credentialEnvironment: "super-secret-key"}, missing: modelEnvironment},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			var output bytes.Buffer
			dependencies := cliDependencies{
				getenv: func(name string) string { return test.env[name] },
				stdout: &output,
			}

			// When
			err := run(context.Background(), []string{"-prompt", "你好"}, dependencies)

			// Then
			if !errors.Is(err, errMissingEnvironment) || !strings.Contains(err.Error(), test.missing) {
				t.Fatalf("run() error = %v, want missing %s", err, test.missing)
			}
			if strings.Contains(err.Error(), "super-secret-key") || strings.Contains(output.String(), "super-secret-key") {
				t.Fatal("CLI output exposed API key")
			}
		})
	}
}

func TestCLI_processFailsSafelyWithoutModelEnvironment(t *testing.T) {
	t.Parallel()

	// Given
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".", "-prompt", "你好")
	command.Env = append(environmentWithoutOpenAIConfig(), credentialEnvironment+"=process-secret")

	// When
	output, err := command.CombinedOutput()

	// Then
	if err == nil {
		t.Fatal("go run succeeded, want missing model failure")
	}
	if !strings.Contains(string(output), modelEnvironment) {
		t.Fatalf("go run output = %q, want missing model name", output)
	}
	if strings.Contains(string(output), "process-secret") {
		t.Fatal("go run output exposed API key")
	}
}

func TestRun_redactsAPIKeyRepeatedByProviderError(t *testing.T) {
	t.Parallel()

	// Given
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		if _, err := writer.Write([]byte(`{"error":{"message":"upstream repeated mock-secret"}}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	environment := map[string]string{
		credentialEnvironment: "mock-secret",
		modelEnvironment:      "gpt-test",
		baseURLEnvironment:    server.URL,
	}

	// When
	err := run(context.Background(), []string{"-prompt", "你好"}, cliDependencies{
		getenv: func(name string) string { return environment[name] },
		stdout: io.Discard,
	})

	// Then
	if err == nil {
		t.Fatal("run() succeeded, want Provider error")
	}
	if strings.Contains(err.Error(), "mock-secret") {
		t.Fatalf("run() error exposed API key: %v", err)
	}
}

func environmentWithoutOpenAIConfig() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, credentialEnvironment+"=") ||
			strings.HasPrefix(entry, modelEnvironment+"=") ||
			strings.HasPrefix(entry, baseURLEnvironment+"=") {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func TestRun_completesToolLoopAgainstMockServer(t *testing.T) {
	t.Parallel()

	// Given
	requests := make(chan cliRequest, 2)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body cliRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- body
		writer.Header().Set("Content-Type", "text/event-stream")
		stream := cliToolStream()
		if requestCount.Add(1) == 2 {
			stream = cliTextStream()
		}
		if _, err := writer.Write([]byte(stream)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	environment := map[string]string{
		credentialEnvironment: "mock-secret",
		modelEnvironment:      "gpt-test",
		baseURLEnvironment:    server.URL,
	}

	// When
	err := run(context.Background(), []string{"-prompt", "请使用 echo 工具"}, cliDependencies{
		getenv: func(name string) string { return environment[name] },
		stdout: &output,
	})
	// Then
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}
	if output.String() != "工具返回：来自 mock\n" {
		t.Fatalf("stdout = %q, want final Assistant text", output.String())
	}
	if strings.Contains(output.String(), "mock-secret") {
		t.Fatal("CLI output exposed API key")
	}
	if requestCount.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requestCount.Load())
	}
	<-requests
	assertCLISecondRequest(t, <-requests)
}

type cliRequest struct {
	Input []struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
	} `json:"input"`
}

func assertCLISecondRequest(t *testing.T, request cliRequest) {
	t.Helper()

	for _, item := range request.Input {
		if item.Type == "function_call_output" && item.CallID == "call_echo" && item.Output == "来自 mock" {
			return
		}
	}
	t.Fatalf("second request = %#v, want echo function_call_output", request)
}

func cliToolStream() string {
	return cliSSE(`{"type":"response.created","response":{"id":"resp_tool","model":"gpt-test"}}`) +
		cliSSE(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_echo","name":"echo","arguments":""}}`) +
		cliSSE(`{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"message\":\"来自 mock\"}"}`) +
		cliSSE(`{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"message\":\"来自 mock\"}"}`) +
		cliSSE(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_echo","name":"echo","arguments":"{\"message\":\"来自 mock\"}"}}`) +
		cliSSE(`{"type":"response.completed","response":{"id":"resp_tool","model":"gpt-test","status":"completed","usage":{}}}`)
}

func cliTextStream() string {
	return cliSSE(`{"type":"response.created","response":{"id":"resp_text","model":"gpt-test"}}`) +
		cliSSE(`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"工具返回：来自 mock"}`) +
		cliSSE(`{"type":"response.output_text.done","output_index":0,"content_index":0,"text":"工具返回：来自 mock"}`) +
		cliSSE(`{"type":"response.completed","response":{"id":"resp_text","model":"gpt-test","status":"completed","usage":{}}}`)
}

func cliSSE(data string) string {
	return "data: " + data + "\n\n"
}
