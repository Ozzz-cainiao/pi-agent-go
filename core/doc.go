// Package agent 实现与模型厂商无关的 Pi Agent Core。
//
// 建议按下面的顺序阅读本包：
//
//  1. types.go：消息、内容块与 AgentContext 等核心数据结构。
//  2. assistant_stream.go：Provider 如何把模型的流式响应交给 Core。
//  3. loop.go 和 loop_runtime.go：一次 Agent Loop 如何跨越多个模型轮次。
//  4. tools.go、tool_runtime.go 和 tool_batch_runtime.go：工具的定义、校验与执行。
//  5. events.go：Core 如何把运行过程暴露给日志、Gateway 或 UI。
//  6. agent.go 及其 agent_* 文件：在低层 Loop 之上管理长期状态的高层 Agent。
//
// 最重要的控制流是：
//
//	UserMessage
//	  -> RunAgentLoop
//	  -> StreamFunc（由 Provider 实现）
//	  -> AssistantMessage
//	  -> ToolCall（如果模型请求工具）
//	  -> ToolResultMessage
//	  -> 下一次 StreamFunc
//	  -> 最终 AssistantMessage
//
// Core 不负责 HTTP、gRPC、Session 持久化或某家模型的 JSON 协议。这些能力应放在
// Provider 或上层云端 Harness 中，因此同一个 Core 可以在 CLI、单元测试和云服务中复用。
package agent
