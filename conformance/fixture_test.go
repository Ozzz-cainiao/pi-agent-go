package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtures_matchCoreBehavior(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatalf("Glob() returned error: %v", err)
	}
	if len(paths) != 6 {
		t.Fatalf("fixture count = %d, want 6", len(paths))
	}
	for _, path := range paths {
		fixture, loadError := LoadFile(path)
		if loadError != nil {
			t.Fatalf("LoadFile(%q): %v", path, loadError)
		}
		t.Run(fixture.Name, func(t *testing.T) {
			actual, runError := Run(context.Background(), fixture)
			if runError != nil {
				t.Fatalf("Run() returned error: %v", runError)
			}
			if compareError := Compare(fixture, actual); compareError != nil {
				t.Fatal(compareError)
			}
		})
	}
}

func TestLoadFile_returnsTypedErrorForUnknownVersion(t *testing.T) {
	t.Parallel()

	// Given
	path := writeFixture(t, `{"version":99,"name":"future","upstream_case":"case","scenario":"simple","input":{},"script":[],"expected":{"events":[],"messages":[],"stop_reason":"stop"}}`)

	// When
	_, err := LoadFile(path)

	// Then
	if !errors.Is(err, ErrUnknownVersion) || !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("LoadFile() error = %v, want typed version error", err)
	}
}

func TestLoadFile_returnsTypedErrorForInvalidEvent(t *testing.T) {
	t.Parallel()

	// Given
	path := writeFixture(t, `{"version":1,"name":"invalid","upstream_case":"case","scenario":"simple","input":{},"script":[],"expected":{"events":["not_an_event"],"messages":[],"stop_reason":"stop"}}`)

	// When
	_, err := LoadFile(path)

	// Then
	if !errors.Is(err, ErrInvalidEvent) || !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("LoadFile() error = %v, want typed event error", err)
	}
	var fixtureError *FixtureError
	if !errors.As(err, &fixtureError) || fixtureError.Field != "expected.events[0]" {
		t.Fatalf("LoadFile() error = %#v, want precise event field", err)
	}
}

func TestLoadFile_rejectsTrailingJSONDocument(t *testing.T) {
	t.Parallel()

	// Given
	path := writeFixture(t, `{"version":1,"name":"trailing","upstream_case":"case","scenario":"simple","input":{},"script":[{"text":"ok","stop_reason":"stop"}],"expected":{"events":[],"messages":[],"stop_reason":"stop"}} {}`)

	// When
	_, err := LoadFile(path)

	// Then
	if !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("LoadFile() error = %v, want trailing-document fixture error", err)
	}
}

func TestCompare_reportsReadableGoldenEventMismatchFromTemporaryCopy(t *testing.T) {
	t.Parallel()

	// Given
	fixture, err := LoadFile("testdata/simple.json")
	if err != nil {
		t.Fatalf("LoadFile() returned error: %v", err)
	}
	actual, err := Run(context.Background(), fixture)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	fixture.Expected.Events[0] = "turn_start"
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() returned error: %v", err)
	}
	temporaryPath := filepath.Join(t.TempDir(), "simple.json")
	if err := os.WriteFile(temporaryPath, encoded, 0o600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	mutated, err := LoadFile(temporaryPath)
	if err != nil {
		t.Fatalf("LoadFile(temporary) returned error: %v", err)
	}

	// When
	err = Compare(mutated, actual)

	// Then
	if !errors.Is(err, ErrFixtureMismatch) {
		t.Fatalf("Compare() error = %v, want ErrFixtureMismatch", err)
	}
	if !strings.Contains(err.Error(), `events[0]`) ||
		!strings.Contains(err.Error(), `want "turn_start"`) ||
		!strings.Contains(err.Error(), `got "agent_start"`) {
		t.Fatalf("Compare() error is not readable: %v", err)
	}
}

func writeFixture(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
	return path
}
