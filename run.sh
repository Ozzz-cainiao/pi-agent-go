#!/usr/bin/env bash

set -euo pipefail

# 建议从终端环境读取 API Key，避免把密钥提交到 Git。
: "${DEEPSEEK_API_KEY:?请先设置 DEEPSEEK_API_KEY}"

export OPENAI_API_KEY="${DEEPSEEK_API_KEY}"
export OPENAI_BASE_URL="https://api.deepseek.com"
export OPENAI_RESPONSES_MODEL="deepseek-v4-flash"

go run ./cmd/pi-agent-example \
  -prompt "${1:-请使用 echo 工具原样返回：你好}"