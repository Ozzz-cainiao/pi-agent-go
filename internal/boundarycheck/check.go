// Package boundarycheck 检查 Core 依赖方向。
package boundarycheck

import (
	"errors"
	"fmt"
	"strings"
)

// ErrProviderDependency 表示 Core 根包的依赖闭包包含 Provider adapter。
var ErrProviderDependency = errors.New("core 根包依赖 Provider")

// ViolationError 描述一条可定位、可分类的仓库边界违规。
type ViolationError struct {
	Kind error
	Path string
}

func (err *ViolationError) Error() string {
	return fmt.Sprintf("%v: %s", err.Kind, err.Path)
}

func (err *ViolationError) Unwrap() error {
	return err.Kind
}

// CheckDependencies 检查 Core 依赖列表中是否包含当前 module 的 Provider。
func CheckDependencies(module string, dependencies []string) error {
	providerRoot := strings.TrimSuffix(module, "/") + "/provider"
	for _, dependency := range dependencies {
		if dependency == providerRoot || strings.HasPrefix(dependency, providerRoot+"/") {
			return &ViolationError{Kind: ErrProviderDependency, Path: dependency}
		}
	}
	return nil
}
