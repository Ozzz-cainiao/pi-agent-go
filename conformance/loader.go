package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

// LoadFile 严格解析并校验一个 conformance fixture。
func LoadFile(path string) (Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixture 路径由测试 runner 调用者显式提供。
	if err != nil {
		return Fixture{}, fmt.Errorf("read conformance fixture: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var fixture Fixture
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, newFixtureError(path, "json", err)
	}
	var trailing json.RawMessage
	trailingError := decoder.Decode(&trailing)
	if !errors.Is(trailingError, io.EOF) {
		if trailingError == nil {
			trailingError = errors.New("multiple JSON documents are not allowed")
		}
		return Fixture{}, newFixtureError(path, "json", trailingError)
	}
	if err := validateFixture(path, fixture); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func validateFixture(path string, fixture Fixture) error {
	if fixture.Version != SchemaVersion {
		return newFixtureError(path, "version", ErrUnknownVersion)
	}
	if fixture.Name == "" {
		return newFixtureError(path, "name", fmt.Errorf("name is required"))
	}
	if fixture.UpstreamCase == "" {
		return newFixtureError(path, "upstream_case", fmt.Errorf("upstream case is required"))
	}
	if !knownScenario(fixture.Scenario) {
		return newFixtureError(path, "scenario", fmt.Errorf("unsupported scenario %q", fixture.Scenario))
	}
	for index, event := range fixture.Expected.Events {
		if !knownEvent(event) {
			return newFixtureError(path, fmt.Sprintf("expected.events[%d]", index), ErrInvalidEvent)
		}
	}
	if _, err := agent.ParseStopReason(fixture.Expected.StopReason); err != nil {
		return newFixtureError(path, "expected.stop_reason", err)
	}
	for index, response := range fixture.Script {
		if response.WaitForCancel || response.StopReason != "" {
			if response.WaitForCancel {
				continue
			}
			if _, err := agent.ParseStopReason(response.StopReason); err != nil {
				return newFixtureError(path, fmt.Sprintf("script[%d].stop_reason", index), err)
			}
		}
	}
	return nil
}

func newFixtureError(path, field string, cause error) error {
	return &FixtureError{Path: path, Field: field, Cause: cause}
}

func knownScenario(scenario string) bool {
	switch scenario {
	case "simple", "tool", "parallel", "queue", "continue", "abort":
		return true
	default:
		return false
	}
}

func knownEvent(event string) bool {
	switch agent.AgentEventKind(event) {
	case agent.AgentEventAgentStart,
		agent.AgentEventAgentEnd,
		agent.AgentEventTurnStart,
		agent.AgentEventTurnEnd,
		agent.AgentEventMessageStart,
		agent.AgentEventMessageUpdate,
		agent.AgentEventMessageEnd,
		agent.AgentEventToolExecutionStart,
		agent.AgentEventToolExecutionUpdate,
		agent.AgentEventToolExecutionEnd:
		return true
	default:
		return false
	}
}
