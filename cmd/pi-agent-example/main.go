package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// OS 信号取消会沿 ctx 传入 Agent，再继续传给 Provider HTTP 请求和 Tool.Execute。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], cliDependencies{getenv: os.Getenv, stdout: os.Stdout})
	stop()
	if err == nil {
		return
	}
	if _, writeError := fmt.Fprintf(os.Stderr, "pi-agent-example: %v\n", err); writeError != nil {
		os.Exit(2)
	}
	os.Exit(1)
}
