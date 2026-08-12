# AGENTS.md

Go 1.26 model-independent Agent Core based on the behavior of pi-agent-core pinned at `2a9b4ebc680053c64e31f635b0b22d5e22564001`.

## Commands

- `go test -race -shuffle=on -count=1 ./...` - run tests
- `go vet ./...` - run standard static checks
- `task check` - run the full quality gate when development tools are installed

## Scope

- Root package `agent` owns public types and Agent Loop behavior.
- `agenttest` will own reusable scripted models and tools.
- `conformance` will own language-neutral compatibility fixtures.
- Root package `agent` must not depend on real model Provider implementations.
- `provider/openairesponses` is an allowed independent adapter subpackage.
- Cloud service transports stay outside this module.

## Conventions

- Write a failing behavior test before production code.
- Put `context.Context` first on cancellable or I/O methods.
- Use typed errors and wrap causes with `%w`.
- Keep Agent state mutation on one owner goroutine.
- Split files by cohesive responsibility. Do not create tiny files only to satisfy a line-count target.
- When a file grows, judge whether it still has one clear owner before deciding to split it.
- Do not add Pi AgentHarness, Session persistence, gRPC, Kubernetes, or cloud Harness services.
