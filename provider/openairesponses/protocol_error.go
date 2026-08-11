package openairesponses

import "fmt"

// ProtocolError 描述不符合 Responses SSE 协议的输入。
type ProtocolError struct {
	Operation string
	Cause     error
}

// Error 返回协议错误说明。
func (protocolError *ProtocolError) Error() string {
	if protocolError.Cause == nil {
		return fmt.Sprintf("openai responses: %s: %v", protocolError.Operation, ErrProtocol)
	}
	return fmt.Sprintf("openai responses: %s: %v", protocolError.Operation, protocolError.Cause)
}

// Unwrap 同时暴露协议错误哨兵和底层错误。
func (protocolError *ProtocolError) Unwrap() []error {
	if protocolError.Cause == nil {
		return []error{ErrProtocol}
	}
	return []error{ErrProtocol, protocolError.Cause}
}
