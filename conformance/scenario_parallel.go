package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
	"github.com/Ozzz-cainiao/pi-agent-go/agenttest"
)

type loopResult struct {
	messages []agent.AgentMessage
	err      error
}

func runParallel(ctx context.Context, fixture Fixture) (Result, error) {
	runContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stream, err := streamFromFixture(fixture)
	if err != nil {
		return Result{}, err
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseTools := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseTools()
	tools := parallelTools(fixture, started, release)
	recorder := &eventRecorder{}
	done := make(chan loopResult, 1)
	go func() {
		messages, runError := agent.RunAgentLoop(
			runContext,
			[]agent.AgentMessage{agent.UserMessage{
				Content: []agent.UserContent{agent.TextContent{Text: fixture.Input.Prompt}},
			}},
			agent.AgentContext{Tools: tools},
			agent.LoopConfig{Stream: stream.Stream, ToolExecution: agent.ToolExecutionModeParallel},
			recorder.sink,
		)
		done <- loopResult{messages: messages, err: runError}
	}()
	for range 2 {
		select {
		case <-started:
		case early := <-done:
			if early.err == nil {
				return Result{}, errors.New("parallel batch completed before both tools started")
			}
			return Result{}, fmt.Errorf("parallel batch completed before both tools started: %w", early.err)
		case <-runContext.Done():
			return Result{}, fmt.Errorf("wait for parallel tools: %w", runContext.Err())
		}
	}
	releaseTools()
	var output loopResult
	select {
	case output = <-done:
	case <-runContext.Done():
		return Result{}, fmt.Errorf("wait for parallel batch completion: %w", runContext.Err())
	}
	if output.err != nil {
		return Result{}, output.err
	}
	if err := ensureScriptConsumed(fixture, stream); err != nil {
		return Result{}, err
	}
	return normalizeResult(recorder.snapshot(), output.messages), nil
}

func parallelTools(fixture Fixture, started chan<- string, release <-chan struct{}) []agent.Tool {
	tools := make([]agent.Tool, 0, len(fixture.Input.ToolResults))
	for name, resultText := range fixture.Input.ToolResults {
		toolName := name
		text := resultText
		tools = append(tools, agenttest.NewScriptedTool(agent.ToolDefinition{
			Name: toolName, Parameters: json.RawMessage(`{"type":"object"}`),
			ExecutionMode: agent.ToolExecutionModeParallel,
		}, func(ctx context.Context, _ agent.ToolCall, _ agent.ToolUpdateFunc) (agent.ToolResult, error) {
			started <- toolName
			select {
			case <-release:
				return textToolResult(text), nil
			case <-ctx.Done():
				return agent.ToolResult{}, ctx.Err()
			}
		}))
	}
	return tools
}
