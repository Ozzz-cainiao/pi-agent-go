package boundarycheck

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckDependencies_rejectsProviderDependency(t *testing.T) {
	const module = "example.com/agent"
	dependencies := []string{
		"context",
		module,
		module + "/provider/openairesponses",
	}

	err := CheckDependencies(module, dependencies)
	if !errors.Is(err, ErrProviderDependency) {
		t.Fatalf("CheckDependencies() error = %v, want ErrProviderDependency", err)
	}
	if !strings.Contains(err.Error(), module+"/provider/openairesponses") {
		t.Fatalf("error = %q, want offending dependency", err)
	}
}

func TestCheckDependencies_acceptsCoreDependencies(t *testing.T) {
	dependencies := []string{"context", "encoding/json", "example.com/agent"}
	if err := CheckDependencies("example.com/agent", dependencies); err != nil {
		t.Fatalf("CheckDependencies() error = %v", err)
	}
}
