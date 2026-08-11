package conformance

import (
	"context"
	"errors"
	"fmt"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

func runQueue(ctx context.Context, fixture Fixture) (result Result, runError error) {
	history, err := messagesFromRecords(fixture.Input.History)
	if err != nil {
		return Result{}, err
	}
	stream, err := streamFromFixture(fixture)
	if err != nil {
		return Result{}, err
	}
	highLevelAgent, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Messages: history},
		SteeringMode: agent.QueueModeOneAtATime,
		LoopConfig:   agent.LoopConfig{Stream: stream.Stream},
	})
	if err != nil {
		return Result{}, err
	}
	defer func() { runError = errors.Join(runError, highLevelAgent.Close()) }()
	recorder := &eventRecorder{}
	unsubscribe, err := highLevelAgent.Subscribe(recorder.subscriber)
	if err != nil {
		return Result{}, err
	}
	defer unsubscribe()
	for _, queued := range fixture.Input.Queued {
		if err := highLevelAgent.Steer(agent.UserMessage{
			Content: []agent.UserContent{agent.TextContent{Text: queued}},
		}); err != nil {
			return Result{}, err
		}
	}
	if err := highLevelAgent.Continue(ctx); err != nil {
		return Result{}, err
	}
	state, err := highLevelAgent.State()
	if err != nil {
		return Result{}, err
	}
	if err := ensureScriptConsumed(fixture, stream); err != nil {
		return Result{}, err
	}
	return normalizeResult(recorder.snapshot(), state.Messages), nil
}

func runAbort(ctx context.Context, fixture Fixture) (result Result, runError error) {
	runContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stream, err := streamFromFixture(fixture)
	if err != nil {
		return Result{}, err
	}
	highLevelAgent, err := agent.NewAgent(agent.AgentOptions{
		LoopConfig: agent.LoopConfig{Stream: stream.Stream},
	})
	if err != nil {
		return Result{}, err
	}
	defer func() { runError = errors.Join(runError, highLevelAgent.Close()) }()
	recorder := &eventRecorder{}
	unsubscribe, err := highLevelAgent.Subscribe(recorder.subscriber)
	if err != nil {
		return Result{}, err
	}
	defer unsubscribe()
	promptDone := make(chan error, 1)
	go func() {
		promptDone <- highLevelAgent.Prompt(runContext, agent.UserMessage{
			Content: []agent.UserContent{agent.TextContent{Text: fixture.Input.Prompt}},
		})
	}()
	select {
	case <-stream.Started():
	case <-runContext.Done():
		return Result{}, fmt.Errorf("wait for abort fixture stream: %w", runContext.Err())
	}
	if err := highLevelAgent.Abort(); err != nil {
		return Result{}, err
	}
	promptError := <-promptDone
	if !errors.Is(promptError, context.Canceled) {
		if promptError == nil {
			return Result{}, errors.New("abort Prompt completed without context cancellation")
		}
		return Result{}, fmt.Errorf("abort Prompt did not preserve context cancellation: %w", promptError)
	}
	state, err := highLevelAgent.State()
	if err != nil {
		return Result{}, err
	}
	if err := ensureScriptConsumed(fixture, stream); err != nil {
		return Result{}, err
	}
	return normalizeResult(recorder.snapshot(), state.Messages), nil
}
