package agent

import "context"

// TransformContextFunc 在转换为 Model 消息之前调整 Agent 消息快照。
type TransformContextFunc func(context.Context, []AgentMessage) ([]AgentMessage, error)

// PrepareNextTurnContext 是一轮完整结束后的只读快照。
type PrepareNextTurnContext struct {
	Message     AssistantMessage
	ToolResults []ToolResultMessage
	Context     AgentContext
	NewMessages []AgentMessage
}

// NextTurnUpdate 描述下一轮需要替换的运行状态。
type NextTurnUpdate struct {
	Context *AgentContext
}

// PrepareNextTurnFunc 在下一轮 Model 调用之前生成运行状态更新。
type PrepareNextTurnFunc func(context.Context, PrepareNextTurnContext) (NextTurnUpdate, error)

func cloneAgentContext(current AgentContext) (AgentContext, error) {
	messages, err := cloneAgentMessages(current.Messages)
	if err != nil {
		return AgentContext{}, err
	}
	cloned := current
	cloned.Messages = messages
	cloned.Tools = append([]Tool(nil), current.Tools...)
	return cloned, nil
}
