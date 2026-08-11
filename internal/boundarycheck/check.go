package boundarycheck

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrProviderDependency = errors.New("core 根包依赖 Provider")
	ErrFileTooLarge       = errors.New("go 文件超过纯代码行限制")
)

type ViolationError struct {
	Kind   error
	Path   string
	Actual int
	Limit  int
}

func (err *ViolationError) Error() string {
	if errors.Is(err.Kind, ErrFileTooLarge) {
		return fmt.Sprintf("%v: %s 有 %d 行，限制为 %d 行", err.Kind, err.Path, err.Actual, err.Limit)
	}
	return fmt.Sprintf("%v: %s", err.Kind, err.Path)
}

func (err *ViolationError) Unwrap() error {
	return err.Kind
}

func CheckDependencies(module string, dependencies []string) error {
	providerRoot := strings.TrimSuffix(module, "/") + "/provider"
	for _, dependency := range dependencies {
		if dependency == providerRoot || strings.HasPrefix(dependency, providerRoot+"/") {
			return &ViolationError{Kind: ErrProviderDependency, Path: dependency}
		}
	}
	return nil
}

func CheckFileSizes(root string, limit int) error {
	source := os.DirFS(root)
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("遍历 %s: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path != root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("计算相对路径 %s: %w", path, err)
		}
		lines, generated, err := countPureLines(source, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		if !generated && lines > limit {
			return &ViolationError{Kind: ErrFileTooLarge, Path: relative, Actual: lines, Limit: limit}
		}
		return nil
	})
}

func ignoredDirectory(name string) bool {
	return name == ".git" || name == ".omo" || name == "vendor"
}

func countPureLines(source fs.FS, path string) (int, bool, error) {
	content, err := fs.ReadFile(source, path)
	if err != nil {
		return 0, false, fmt.Errorf("读取 %s: %w", path, err)
	}

	lines := 0
	generated := false
	beforePackage := true
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if beforePackage && strings.HasPrefix(trimmed, "// Code generated ") && strings.HasSuffix(trimmed, " DO NOT EDIT.") {
			generated = true
		}
		if strings.HasPrefix(trimmed, "package ") {
			beforePackage = false
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			lines++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, false, fmt.Errorf("读取 %s: %w", path, err)
	}
	return lines, generated, nil
}
