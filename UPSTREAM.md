# Upstream compatibility reference

The behavioral reference for this project is:

- Upstream repository: `https://github.com/earendil-works/pi.git`
- Pinned commit: `3709bef75e4f89508fe66f0e9d124c639de0695c`
- Upstream package version at the pinned commit: `@earendil-works/pi-agent-core@0.84.1`

The Go implementation targets behavioral compatibility rather than source or API compatibility. Event ordering, transcript ordering, tool execution semantics, cancellation, queue behavior, and terminal outcomes are compatibility-sensitive.

The upstream `packages/agent/src/harness` implementation is outside the initial port scope.
