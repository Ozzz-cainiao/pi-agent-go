package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
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
