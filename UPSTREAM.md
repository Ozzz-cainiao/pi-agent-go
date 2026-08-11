# Upstream compatibility reference

The behavioral reference for this project is:

- Upstream repository: `https://github.com/earendil-works/pi.git`
- Pinned commit: `2a9b4ebc680053c64e31f635b0b22d5e22564001`
- Upstream package version at the pinned commit: `@earendil-works/pi-agent-core@0.84.1`

The Go implementation targets behavioral compatibility rather than source or API compatibility. Event ordering, transcript ordering, tool execution semantics, cancellation, queue behavior, and terminal outcomes are compatibility-sensitive.

The root `agent` Core remains model-provider independent. The `provider/openairesponses` adapter is allowed as a separate package because it does not add a Provider dependency to the root Core.

The upstream `packages/agent/src/harness` implementation is outside the initial port scope. Session persistence, gRPC transports, Kubernetes deployment, and cloud Harness services are also outside this port scope.
