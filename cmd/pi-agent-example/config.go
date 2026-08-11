package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const (
	credentialEnvironment = "OPENAI_API_KEY" //nolint:gosec // 这是环境变量名称，不包含凭据值。
	modelEnvironment      = "OPENAI_RESPONSES_MODEL"
	baseURLEnvironment    = "OPENAI_BASE_URL"
)

var (
	errMissingEnvironment = errors.New("pi-agent-example: missing environment")
	errMissingPrompt      = errors.New("pi-agent-example: missing prompt")
)

type cliConfig struct {
	apiKey  string
	model   string
	baseURL string
	prompt  string
}

type missingEnvironmentError struct {
	name string
}

func (missingError *missingEnvironmentError) Error() string {
	return fmt.Sprintf("缺少必需环境变量 %s", missingError.name)
}

func (missingError *missingEnvironmentError) Unwrap() error {
	return errMissingEnvironment
}

func parseCLIConfig(args []string, getenv func(string) string) (cliConfig, error) {
	flags := flag.NewFlagSet("pi-agent-example", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	prompt := flags.String("prompt", "", "发送给 Agent 的用户文本")
	if err := flags.Parse(args); err != nil {
		return cliConfig{}, fmt.Errorf("解析命令行参数: %w", err)
	}
	config := cliConfig{
		apiKey:  strings.TrimSpace(getenv(credentialEnvironment)),
		model:   strings.TrimSpace(getenv(modelEnvironment)),
		baseURL: strings.TrimSpace(getenv(baseURLEnvironment)),
		prompt:  strings.TrimSpace(*prompt),
	}
	if config.apiKey == "" {
		return cliConfig{}, &missingEnvironmentError{name: credentialEnvironment}
	}
	if config.model == "" {
		return cliConfig{}, &missingEnvironmentError{name: modelEnvironment}
	}
	if config.prompt == "" {
		return cliConfig{}, errMissingPrompt
	}
	return config, nil
}
