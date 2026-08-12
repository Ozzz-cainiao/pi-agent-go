// Package main 提供可由质量门调用的仓库边界检查命令。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/Ozzz-cainiao/pi-agent-go/internal/boundarycheck"
)

func main() {
	log.SetFlags(0)
	if err := run(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "边界检查失败:", err)
		os.Exit(1)
	}
	log.Print("边界检查通过")
}

func run(ctx context.Context) error {
	moduleCommand := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Path}}\n{{.Dir}}")
	moduleInfo, err := commandLines(moduleCommand, "读取模块信息")
	if err != nil {
		return err
	}
	if len(moduleInfo) != 2 {
		return fmt.Errorf("解析模块信息: 预期 2 行，得到 %d 行", len(moduleInfo))
	}
	module, root := moduleInfo[0], moduleInfo[1]

	dependencyCommand := exec.CommandContext(ctx, "go", "list", "-deps", ".")
	dependencyCommand.Dir = root
	dependencies, err := commandLines(dependencyCommand, "读取 Core 依赖")
	if err != nil {
		return err
	}
	return boundarycheck.CheckDependencies(module, dependencies)
}

func commandLines(command *exec.Cmd, action string) ([]string, error) {
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", action, err, strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}
