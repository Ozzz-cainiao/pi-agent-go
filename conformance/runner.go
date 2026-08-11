package conformance

import (
	"context"
	"fmt"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

// Run 执行 fixture 指定的 Core 场景并返回语言无关结果。
func Run(ctx context.Context, fixture Fixture) (Result, error) {
	if err := validateFixture("<memory>", fixture); err != nil {
		return Result{}, err
	}
	switch fixture.Scenario {
	case "simple", "tool":
		return runLowLevel(ctx, fixture, agent.ToolExecutionModeSequential)
	case "parallel":
		return runParallel(ctx, fixture)
	case "queue":
		return runQueue(ctx, fixture)
	case "continue":
		return runContinuation(ctx, fixture)
	case "abort":
		return runAbort(ctx, fixture)
	default:
		return Result{}, fmt.Errorf("run unsupported fixture scenario %q: %w", fixture.Scenario, ErrInvalidFixture)
	}
}
