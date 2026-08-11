package agent

import (
	"errors"
	"fmt"
)

// ErrUnsupportedSnapshotValue 表示事件快照包含无法安全复制的值。
var ErrUnsupportedSnapshotValue = errors.New("agent: unsupported snapshot value")

// SnapshotCloneError 描述无法安全复制的事件值位置与类型。
type SnapshotCloneError struct {
	Path string
	Type string
}

// Error 返回事件快照复制错误说明。
func (cloneError *SnapshotCloneError) Error() string {
	return fmt.Sprintf(
		"agent: cannot safely clone snapshot value at %s with type %s",
		cloneError.Path,
		cloneError.Type,
	)
}

// Unwrap 暴露不支持的快照值哨兵错误。
func (cloneError *SnapshotCloneError) Unwrap() error {
	return ErrUnsupportedSnapshotValue
}
