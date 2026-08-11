package agent

import "fmt"

// ModelID 是 Core 用来标识当前模型的稳定轻量标识。
type ModelID string

// UnknownModelID 表示尚未绑定具体模型。
const UnknownModelID ModelID = "unknown"

// ThinkingLevel 表示请求模型采用的推理强度。
type ThinkingLevel string

const (
	ThinkingLevelOff     ThinkingLevel = "off"
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	ThinkingLevelLow     ThinkingLevel = "low"
	ThinkingLevelMedium  ThinkingLevel = "medium"
	ThinkingLevelHigh    ThinkingLevel = "high"
	ThinkingLevelXHigh   ThinkingLevel = "xhigh"
	ThinkingLevelMax     ThinkingLevel = "max"
)

// AgentState 是高层 Agent 当前状态的只读快照。
type AgentState struct {
	SystemPrompt     string
	Model            ModelID
	ThinkingLevel    ThinkingLevel
	Tools            []Tool
	Messages         []AgentMessage
	IsStreaming      bool
	StreamingMessage AgentMessage
	PendingToolCalls map[string]struct{}
	ErrorMessage     string
}

// AgentInitialState 是构造时允许设置的稳定状态。
type AgentInitialState struct {
	SystemPrompt  string
	Model         ModelID
	ThinkingLevel ThinkingLevel
	Tools         []Tool
	Messages      []AgentMessage
}

// AgentOptions 配置高层 Agent。
type AgentOptions struct {
	InitialState *AgentInitialState
}

func defaultAgentState() AgentState {
	return AgentState{
		Model:            UnknownModelID,
		ThinkingLevel:    ThinkingLevelOff,
		Tools:            []Tool{},
		Messages:         []AgentMessage{},
		PendingToolCalls: map[string]struct{}{},
	}
}

func initialAgentState(initial *AgentInitialState) (AgentState, error) {
	if initial == nil {
		return defaultAgentState(), nil
	}
	state := defaultAgentState()
	state.SystemPrompt = initial.SystemPrompt
	state.Model = initial.Model
	state.ThinkingLevel = initial.ThinkingLevel
	state.Tools = append([]Tool(nil), initial.Tools...)
	messages, err := cloneAgentMessages(initial.Messages)
	if err != nil {
		return AgentState{}, fmt.Errorf("clone initial agent state: %w", err)
	}
	state.Messages = messages
	if state.Model == "" {
		state.Model = UnknownModelID
	}
	if state.ThinkingLevel == "" {
		state.ThinkingLevel = ThinkingLevelOff
	}
	return state, nil
}

func cloneAgentState(state AgentState) (AgentState, error) {
	cloned := state
	cloned.Tools = append([]Tool(nil), state.Tools...)
	if state.Messages != nil {
		messages, err := cloneAgentMessages(state.Messages)
		if err != nil {
			return AgentState{}, fmt.Errorf("clone state messages: %w", err)
		}
		cloned.Messages = messages
	}
	if state.StreamingMessage != nil {
		streaming, cloneError := cloneAgentMessages([]AgentMessage{state.StreamingMessage})
		if cloneError != nil {
			return AgentState{}, fmt.Errorf("clone streaming message: %w", cloneError)
		}
		cloned.StreamingMessage = streaming[0]
	}
	cloned.PendingToolCalls = clonePendingToolCalls(state.PendingToolCalls)
	return cloned, nil
}

func clonePendingToolCalls(calls map[string]struct{}) map[string]struct{} {
	if calls == nil {
		return nil
	}
	cloned := make(map[string]struct{}, len(calls))
	for callID := range calls {
		cloned[callID] = struct{}{}
	}
	return cloned
}
