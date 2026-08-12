package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
	"github.com/Ozzz-cainiao/pi-agent-go/provider/openairesponses"
)

var errNoAssistantText = errors.New("pi-agent-example: no assistant text")

type cliDependencies struct {
	getenv func(string) string
	stdout io.Writer
}

func run(ctx context.Context, args []string, dependencies cliDependencies) (runError error) {
	// 这个示例展示最短的真实接入路径：读取配置 -> 创建 Provider -> 注入高层 Agent
	// -> 提交 UserMessage -> 从 AgentState 读取最终 Assistant 文本。
	config, err := parseCLIConfig(args, dependencies.getenv)
	if err != nil {
		return err
	}
	defer func() {
		runError = redactCredential(runError, config.apiKey)
	}()
	provider, err := openairesponses.New(openairesponses.Config{
		APIKey: config.apiKey, BaseURL: config.baseURL, Model: config.model,
	})
	if err != nil {
		return fmt.Errorf("创建 Responses Provider: %w", err)
	}
	// Provider.Stream 满足 Core 的 StreamFunc；Core 并不知道下游使用的是 HTTP Responses API。
	highLevelAgent, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{
			SystemPrompt: "你是一个演示助手。需要原样返回内容时可以调用 echo 工具。",
			Model:        agent.ModelID(config.model),
			Tools:        []agent.Tool{echoTool{}},
		},
		LoopConfig: agent.LoopConfig{Stream: provider.Stream},
	})
	if err != nil {
		return fmt.Errorf("创建 Agent: %w", err)
	}
	defer func() {
		runError = errors.Join(runError, highLevelAgent.Close())
	}()
	// UserMessage 是一整条 transcript 消息；TextContent 是它内部的一段内容块。
	userMessage := agent.UserMessage{
		Content: []agent.UserContent{agent.TextContent{Text: config.prompt}},
	}
	if err := highLevelAgent.Prompt(ctx, userMessage); err != nil {
		return fmt.Errorf("执行 Agent: %w", err)
	}
	state, err := highLevelAgent.State()
	if err != nil {
		return fmt.Errorf("读取 Agent 状态: %w", err)
	}
	text, err := lastAssistantText(state.Messages)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(dependencies.stdout, text); err != nil {
		return fmt.Errorf("写入输出: %w", err)
	}
	return nil
}

type redactedError struct {
	cause   error
	message string
}

func (redaction *redactedError) Error() string {
	return redaction.message
}

func (redaction *redactedError) Unwrap() error {
	return redaction.cause
}

func redactCredential(err error, credential string) error {
	if err == nil || credential == "" || !strings.Contains(err.Error(), credential) {
		return err
	}
	return &redactedError{
		cause:   err,
		message: strings.ReplaceAll(err.Error(), credential, "[REDACTED]"),
	}
}

func lastAssistantText(messages []agent.AgentMessage) (string, error) {
	for index := len(messages) - 1; index >= 0; index-- {
		message, ok := messages[index].(agent.AssistantMessage)
		if !ok {
			continue
		}
		var text strings.Builder
		for _, content := range message.Content {
			if block, isText := content.(agent.TextContent); isText {
				text.WriteString(block.Text)
			}
		}
		if text.Len() > 0 {
			return text.String(), nil
		}
	}
	return "", errNoAssistantText
}
