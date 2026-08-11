# AGENTS.md

Go 1.23+ model-independent Agent Core based on the behavior of pi-agent-core.

## Commands

- `go test -race -shuffle=on -count=1 ./...` - run tests
- `go vet ./...` - run standard static checks
- `task check` - run the full quality gate when development tools are installed

## Scope

- Root package `agent` owns public types and Agent Loop behavior.
- `agenttest` will own reusable scripted models and tools.
- `conformance` will own language-neutral compatibility fixtures.
- Real model providers and cloud service transports stay outside this module.

## Conventions

- Write a failing behavior test before production code.
- Put `context.Context` first on cancellable or I/O methods.
- Use typed errors and wrap causes with `%w`.
- Keep Agent state mutation on one owner goroutine.
- Keep each non-generated Go file below 250 pure lines.
- Do not add Pi AgentHarness, persistence, gRPC, or provider implementations.
