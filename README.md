# pi-agent-go

`pi-agent-go` 是一个教学驱动的 Go 语言 Agent Core 项目。项目以 Pi Agent Core 的稳定行为为参考，使用符合 Go 习惯的类型、取消和并发模型重新实现。

## 当前状态

项目目前只有工程骨架。Agent Core 的测试和实现将通过结对学习逐步加入，核心代码由仓库所有者亲自编写。

## 上游基准

- Repository: <https://github.com/earendil-works/pi>
- Commit: `3709bef75e4f89508fe66f0e9d124c639de0695c`
- Package: `packages/agent`
- License: MIT

主要参考文件：

- `packages/agent/src/types.ts`
- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/agent.ts`
- `packages/agent/src/stream-fn.ts`
- `packages/agent/test/agent-loop.test.ts`
- `packages/agent/test/agent.test.ts`

## 目标范围

- 消息与内容块
- 模型流事件
- Agent 事件
- Model -> Tool -> Model 循环
- 工具参数校验、串行和并行执行
- Hook、steering、follow-up 与 terminate
- `context.Context` 取消传播
- Agent 生命周期和状态快照
- TypeScript/Go 行为兼容测试

## 暂不包含

- Pi `AgentHarness`
- Session Tree、Lane 和 JSONL 持久化
- Compaction、Skills 和 Prompt Template
- CLI/TUI 和本地文件工具
- 真实模型 Provider
- gRPC、Gateway、Kubernetes 和云端 Harness 服务

## 开发方法

每个行为采用 Red -> Green -> Refactor：

1. 从 Pi 测试中提取一个行为。
2. 先编写会失败的 Go 测试。
3. 运行测试并确认失败原因正确。
4. 编写最小实现使测试通过。
5. 重构并运行完整质量检查。

## 本地命令

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
```

安装 `go-task`、`gofumpt`、`golangci-lint v2` 和 `nilaway` 后，也可以运行：

```bash
task check
```
