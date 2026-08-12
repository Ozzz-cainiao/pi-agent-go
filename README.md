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

目录层级如下：

```text
pi-agent-go/
├── *.go                         # package agent：Provider 无关的 Core
├── agenttest/                   # 可复用的测试 Stream、Tool 和 Clock
├── conformance/                 # TypeScript/Go 行为兼容夹具
├── provider/openairesponses/    # OpenAI Responses API adapter
├── cmd/pi-agent-example/        # 可运行的 API 调用示例
└── internal/boundarycheck/      # Core 依赖方向检查
```

root `agent` 包内按完整职责组织源码，而不是按单个小函数拆文件：

| 文件 | 职责 |
| --- | --- |
| `types.go` | Content、Message、Usage、StopReason、AgentContext |
| `assistant_stream.go` | Provider 向 Core 发送的 Assistant 流事件 |
| `events.go` | Core 向上层发送的 Agent 生命周期事件 |
| `snapshots.go` | 消息和事件对象图的防御性深复制 |
| `loop.go` | 低层 Agent Loop 入口、配置和 continuation |
| `loop_runtime.go` | 每轮 Model → Tool → Model 状态机与 turn 控制 |
| `tools.go` | Tool 公共契约、参数校验和 Hook 类型 |
| `tool_runtime.go` | 单个工具的准备、执行、Hook 和 update 生命周期 |
| `tool_batch_runtime.go` | 串行/并行工具批次调度与稳定结果顺序 |
| `agent.go` | 高层 Agent 公共配置、状态和运行入口 |
| `agent_runtime.go` | owner goroutine、状态命令和失败闭合 |
| `agent_queues.go` | steering/follow-up 队列 |
| `agent_subscription.go` | 事件订阅与串行分发 |

推荐先读 `types.go`，再读 `assistant_stream.go`、`events.go`、`loop.go`、`loop_runtime.go`、`tools.go`、`tool_runtime.go`，最后阅读高层 `agent.go`。Provider 与云端 Harness 都是 Core 的外层。

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

## 学习顺序

建议先按上面的职责顺序阅读当前代码；需要理解演进过程时，再按下面的提交顺序回看。表中的旧文件名描述的是当时提交里的组织方式，可使用 `git show <提交>` 查看。

| 提交 | 学习主题 | 对应的 Pi 源行为 | Go 文件/测试 | 学习目的 |
| --- | --- | --- | --- | --- |
| `3b13d54` → `50c7049` | 消息、内容、stop reason、快照 | `types.ts` 的 Message/Content 联合类型与上下文消息 | `content.go`、`message.go`、`context.go` 及同名测试 | 先掌握循环传递的数据模型和不可变快照 |
| `aa9ade7` | Tool 接口 | `types.ts` 的 AgentTool 与 ToolResult | `tool.go`、`tool_test.go` | 理解模型工具调用与 Go 接口的边界 |
| `004ceab` → `d931c7c` | 最小循环、取消、流输出 | `agent-loop.ts` 的单轮调用；`stream-fn.ts` 的流边界 | `agent_loop.go`、`stream.go`、`agent_loop_stream_cancel_test.go` | 从最小调用链理解取消和增量事件传播 |
| `35665d9` → `12d79ad` | Model → Tool → Model | `agent-loop.ts` 的工具查找、执行和 stop reason 分支 | `tool_execution.go`、`tool_lookup.go`、`agent_loop_tool_loop_test.go` | 跑通最核心的两轮工具闭环 |
| `8da2a8f` | 最小 Responses adapter | `stream-fn.ts` 所代表的 Provider/Core 隔离点 | `provider/openairesponses` 的基础请求测试 | 理解 Core 如何通过 adapter 接真实模型 |
| `815d4c8` | 固定上游与测试拆分 | 固定 Pi `2a9b4eb`；按 `agent-loop.test.ts` 的停止、取消、工具循环场景组织测试 | `UPSTREAM.md`、`README.md`、`agent_loop_*_test.go` | 确立可复现行为基准，并让每类失败可单独定位 |
| `399dbd4` | 基线 gofumpt 收敛 | 不改变 Pi 运行时行为；保护已完成的消息与 Tool 类型语义 | `stop_reason_test.go`、`tool_test.go` | 学习只做机械格式提交，避免格式噪声混入功能差异 |
| `540c69d` | 基线静态检查收敛 | 不改变 Pi 行为；检查已有 Agent Loop 和 Responses adapter 的错误处理与分支完整性 | `agent_loop.go`、`provider/openairesponses/provider*.go`、`request.go` | 在继续移植前建立零 lint 的可信起点 |
| `9189bb4` | LoopConfig、Clock、自定义消息 | `types.ts` 的 custom message；loop options 与时间戳 | `loop_config.go`、`custom_message_test.go`、`loop_config_test.go` | 把环境依赖变成可注入、可测试配置 |
| `4b4ad54` → `4da9d50` | 完整 Assistant 流事件和安全快照 | `types.ts` 的 start/text/thinking/toolcall/done/error 事件 | `assistant_event_*.go`、`assistant_event_*_test.go`、`snapshot_graph_clone.go` | 理解事件累积和任意对象图的防御性复制 |
| `02044ed` → `542df14` | Agent/turn/message/tool 生命周期 | `agent-loop.ts` 的 AgentEvent 顺序和工具 update | `agent_event*.go`、`agent_lifecycle_test.go`、`agent_tool_lifecycle_test.go` | 掌握成功和失败路径如何闭合事件 |
| `53ef84d` | Continue 与上下文转换 Hook | `agentLoopContinue`、`transformContext`、`convertToLlm` | `continuation.go`、`context_hook*.go`、`continuation_test.go` | 理解继续执行和发送给模型的上下文边界 |
| `aa3cfbb` | 工具参数准备与 Schema 校验 | 工具执行前的 prepare → validate 顺序 | `jsonschema_validator.go`、`tool_preparation.go`、`tool_argument_validation_test.go` | 在执行副作用前解析并拒绝非法参数 |
| `8d51fdd` | before/after Tool Hook | 工具调用前阻断/改参和调用后改写结果 | `tool_hook*.go`、`tool_hook_test.go` | 理解工具策略扩展点和调用顺序 |
| `f695746` | partial/late update、terminate | 工具 update 生命周期和全批次 terminate 规则 | `tool_update_gate.go`、`tool_update_late_test.go`、`tool_terminate_test.go` | 掌握异步更新关闭与批次停止语义 |
| `76a9557` | 确定性串行/并行工具批次 | 允许并行时并发执行，但 transcript 保持 source order | `tool_batch*.go`、`tool_batch_*_test.go` | 区分完成顺序与稳定输出顺序 |
| `f38e0fd` | turn 停止控制 | max turns、stop-after-turn、queued message 边界 | `turn_control.go`、`turn_control_test.go` | 理解所有终止分支和无限循环保护 |
| `866586f` | 高层 Agent state | `agent.ts` 的配置、状态和不可变读取 | `agent_owner.go`、`agent_state.go`、`agent_state_*_test.go` | 学习 owner goroutine 与状态快照 |
| `d3966d9` → `7558f8a` | 订阅、运行守卫、队列、失败闭合 | `agent.ts` 的 subscribe/prompt/continue/abort/reset/steer/followUp | `agent_subscription.go`、`agent_run_control.go`、`agent_queue*.go`、相关测试 | 组合出可供服务层调用的高层 Agent |
| `f5627d4` → `fc1dc57` | Responses 文本/工具/Usage/错误 | Provider 把 SSE 映射成 Core 事件和最终消息 | `provider/openairesponses/*_decoder.go`、`provider_*_test.go` | 理解真实协议如何适配稳定 Core 契约 |
| `fa61f9e` | 可运行 CLI | 用真实 Provider 驱动高层 Agent 和 echo 工具闭环 | `cmd/pi-agent-example` 及 mock/live 测试 | 从外部入口验证完整调用链 |
| `045073f` | 跨语言行为兼容夹具 | 固化 simple/tool/parallel/queue/continue/abort 的可比较结果 | `agenttest`、`conformance`、`conformance/testdata` | 用稳定 fixture 发现后续实现漂移 |

阅读某一步时，可使用 `git show <提交>` 查看完整增量，再运行表中相关测试。上游固定版本和对齐原则另见 `UPSTREAM.md`。

## Pi 行为对齐清单

- [x] Message、Content、Usage、StopReason 与自定义消息转换
- [x] Assistant 流事件和 Agent 生命周期事件的顺序、快照隔离、失败闭合
- [x] Model → Tool → Model 多轮循环和 continuation
- [x] 工具查找、JSON Schema 参数校验、Hook、update、terminate
- [x] 工具串行/并行策略；完成事件按完成顺序，结果消息按调用源顺序
- [x] max turns、停止原因、`context.Context` 取消和无限循环保护
- [x] 高层 Agent 状态、订阅、Prompt/Continue/Abort/Reset 和单运行守卫
- [x] steering/follow-up 两种队列模式及注入边界
- [x] Responses SSE 的文本、function call、Usage、远端错误和取消映射
- [x] simple/tool/parallel/queue/continue/abort 的版本化 conformance fixtures

## 有意保留的 Go 差异

本项目追求行为兼容，而不是逐行翻译或 API 形状一致：

- `StreamFunc` 必须显式注入。Core 不读取全局 Provider，也不会根据模型名称隐式选择 Provider，因此云端服务可以明确控制网络客户端、凭据和测试替身。
- Go API 使用普通的返回值和 typed `error`，并支持 `errors.Is`/`errors.As`；它不会复制 TypeScript 的 throw/reject 形状。
- 取消通过 `context.Context` 逐层传递，而不是 JavaScript `AbortSignal`。
- 高层可变状态由单一 owner goroutine 串行化；事件和状态对外提供防御性快照，而不是依赖 JavaScript 单线程语义。
- 当前只保留轻量 `ModelID`，Responses Provider 位于独立 adapter 包；多 Provider 路由不属于本次核心移植。

## 架构边界

`task boundary` 会检查一条硬约束：

1. root `agent` Core 的依赖闭包不能包含当前 module 下的 `provider/...`。

检查器位于 `internal/boundarycheck`，命令入口位于 `cmd/check-boundaries`，并已接入 `task check`。文件规模不再设置机械行数上限：新增代码应按完整职责组织；只有当一个文件无法用一个清晰职责描述时才拆分，不能为了满足行数目标制造零散小文件。

## 新开发者快速验证

项目使用 Go `1.26.5`。准备 `go-task`、`gofumpt`、`golangci-lint v2`、`nilaway` 后，在仓库根目录运行：

```bash
go version
task check
go test ./cmd/pi-agent-example -run TestRun_completesToolLoopAgainstMockServer -v
```

前两个命令完成格式、边界、静态检查和 race 测试；第三个命令在本机 mock server 上完成一次 Model → Tool → Model 闭环，不需要 API Key，也不访问外网。

## 本地命令

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
go run ./cmd/check-boundaries
```

安装 `go-task`、`gofumpt`、`golangci-lint v2` 和 `nilaway` 后，也可以运行：

```bash
task check
task boundary
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
