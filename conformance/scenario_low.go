package conformance

import (
	"context"
	"fmt"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
	"github.com/Ozzz-cainiao/pi-agent-go/agenttest"
)

func runLowLevel(
	ctx context.Context,
	fixture Fixture,
	mode agent.ToolExecutionMode,
) (Result, error) {
	stream, err := streamFromFixture(fixture)
	if err != nil {
		return Result{}, err
	}
	tools := toolsFromFixture(fixture, mode)
	recorder := &eventRecorder{}
	prompts := []agent.AgentMessage{}
	if fixture.Input.Prompt != "" {
		prompts = append(prompts, agent.UserMessage{
			Content: []agent.UserContent{agent.TextContent{Text: fixture.Input.Prompt}},
		})
	}
	messages, err := agent.RunAgentLoop(
		ctx,
		prompts,
		agent.AgentContext{Tools: tools},
		agent.LoopConfig{
			Stream: stream.Stream, ToolExecution: mode,
			Clock: agenttest.Clock(time.Unix(1, 0), time.Second),
		},
		recorder.sink,
	)
	if err != nil {
		return Result{}, err
	}
	if err := ensureScriptConsumed(fixture, stream); err != nil {
		return Result{}, err
	}
	return normalizeResult(recorder.snapshot(), messages), nil
}

func runContinuation(ctx context.Context, fixture Fixture) (Result, error) {
	history, err := messagesFromRecords(fixture.Input.History)
	if err != nil {
		return Result{}, err
	}
	stream, err := streamFromFixture(fixture)
	if err != nil {
		return Result{}, err
	}
	recorder := &eventRecorder{}
	messages, err := agent.ContinueAgentLoop(
		ctx,
		agent.AgentContext{Messages: history},
		agent.LoopConfig{Stream: stream.Stream},
		recorder.sink,
	)
	if err != nil {
		return Result{}, err
	}
	if err := ensureScriptConsumed(fixture, stream); err != nil {
		return Result{}, err
	}
	return normalizeResult(recorder.snapshot(), messages), nil
}

func ensureScriptConsumed(fixture Fixture, stream *agenttest.ScriptedStream) error {
	if calls := len(stream.Calls()); calls != len(fixture.Script) {
		return fmt.Errorf("fixture %q Model calls = %d, want %d", fixture.Name, calls, len(fixture.Script))
	}
	return nil
}
