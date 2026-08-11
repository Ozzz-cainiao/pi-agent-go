# pi-agent-go

`pi-agent-go` 是一个教学驱动的 Go 语言 Agent Core 项目。项目以 Pi Agent Core 的稳定行为为参考，使用符合 Go 习惯的类型、取消和并发模型重新实现。

## 当前状态

项目已经包含可运行的 Agent Core、OpenAI Responses Provider 和一个最小命令行示例。功能仍按独立提交逐步扩展，便于沿提交历史学习每个行为。

## 上游基准

- Repository: <https://github.com/earendil-works/pi>
- Commit: `2a9b4ebc680053c64e31f635b0b22d5e22564001`
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

## 模块边界

root `agent` Core 只定义公共类型与 Agent Loop 行为，不依赖任何实际模型 Provider。`provider/openairesponses` 是允许存在的独立 adapter 子包，用于在 Core 外部连接 OpenAI Responses API。

## 暂不包含

- Pi `AgentHarness`
- Session Tree、Lane 和 JSONL 持久化
- Compaction、Skills 和 Prompt Template
- 生产级 CLI/TUI 和本地文件工具；仓库只提供 API 调用示例 CLI
- root `agent` Core 内的实际模型 Provider 依赖
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

## Responses Provider 调用示例

`cmd/pi-agent-example` 使用高层 `Agent` 调用 Responses API，并注册了一个简单的 `echo` 工具，用于展示 Model -> Tool -> Model 闭环。

先设置当前账号可用的模型和 API Key：

```bash
export OPENAI_API_KEY='你的 API Key'
export OPENAI_RESPONSES_MODEL='你的 Responses 模型名'
```

如果使用兼容 Responses API 的测试服务或公司代理，可以额外设置：

```bash
export OPENAI_BASE_URL='http://127.0.0.1:8080/v1'
```

随后运行：

```bash
go run ./cmd/pi-agent-example -prompt '请使用 echo 工具原样返回：你好'
```

缺少 `OPENAI_API_KEY` 或 `OPENAI_RESPONSES_MODEL` 时，命令会快速失败并只显示缺少的环境变量名。示例不会打印 API Key。不要把真实 Key 写入源码、测试、README 或提交到 Git；本地开发应通过 shell 环境或公司的 Secret 管理系统注入。

不访问外网的 mock 闭环测试：

```bash
go test ./cmd/pi-agent-example -run TestRun_completesToolLoopAgainstMockServer -v
```

真实 API 冒烟测试：

```bash
go test ./cmd/pi-agent-example -run TestCLI_liveProvider -v
```

真实测试仅在 `OPENAI_API_KEY` 和 `OPENAI_RESPONSES_MODEL` 都存在时执行，否则会明确显示 `SKIP`。测试输出同样不会包含 API Key。
