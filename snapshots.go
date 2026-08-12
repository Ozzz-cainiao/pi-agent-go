package agent

import (
	"errors"
	"fmt"
	"reflect"
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

type snapshotVisit struct {
	kind     reflect.Kind
	typeOf   reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

type snapshotCloner struct {
	visited map[snapshotVisit]reflect.Value
}

func newSnapshotCloner() *snapshotCloner {
	return &snapshotCloner{visited: make(map[snapshotVisit]reflect.Value)}
}

func (cloner *snapshotCloner) cloneValue(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if !value.IsValid() {
		return value, nil
	}

	switch value.Kind() {
	case reflect.Interface:
		return cloner.cloneInterface(value, path)
	case reflect.Map:
		return cloner.cloneMap(value, path)
	case reflect.Slice:
		return cloner.cloneSlice(value, path)
	case reflect.Array:
		return cloner.cloneArray(value, path)
	case reflect.Pointer:
		return cloner.clonePointer(value, path)
	case reflect.Struct:
		return cloner.cloneStruct(value, path)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return reflect.Value{}, newSnapshotCloneError(path, value.Type())
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return value, nil
	}

	return value, nil
}

func (cloner *snapshotCloner) cloneInterface(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}

	clonedValue, err := cloner.cloneValue(value.Elem(), path)
	if err != nil {
		return reflect.Value{}, err
	}
	cloned := reflect.New(value.Type()).Elem()
	cloned.Set(clonedValue)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneMap(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
	cloner.visited[visit] = cloned
	iterator := value.MapRange()
	for iterator.Next() {
		clonedKey, err := cloner.cloneValue(iterator.Key(), path+"[key]")
		if err != nil {
			return reflect.Value{}, err
		}
		clonedValue, err := cloner.cloneValue(iterator.Value(), path+"[value]")
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.SetMapIndex(clonedKey, clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) cloneSlice(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{
		kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer(),
		length: value.Len(), capacity: value.Cap(),
	}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
	cloner.visited[visit] = cloned
	for index := range value.Len() {
		clonedValue, err := cloner.cloneValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Index(index).Set(clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) cloneArray(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	cloned := reflect.New(value.Type()).Elem()
	for index := range value.Len() {
		clonedValue, err := cloner.cloneValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Index(index).Set(clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) clonePointer(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.New(value.Type().Elem())
	cloner.visited[visit] = cloned
	clonedValue, err := cloner.cloneValue(value.Elem(), path+"*")
	if err != nil {
		return reflect.Value{}, err
	}
	cloned.Elem().Set(clonedValue)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneStruct(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	cloned := reflect.New(value.Type()).Elem()
	cloned.Set(value)
	for index := range value.NumField() {
		fieldType := value.Type().Field(index)
		fieldPath := path + "." + fieldType.Name
		if !fieldType.IsExported() {
			if containsMutableValue(value.Field(index)) {
				return reflect.Value{}, newSnapshotCloneError(fieldPath, fieldType.Type)
			}

			continue
		}

		clonedField, err := cloner.cloneValue(value.Field(index), fieldPath)
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Field(index).Set(clonedField)
	}

	return cloned, nil
}

func containsMutableValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}

	switch value.Kind() {
	case reflect.Interface:
		return !value.IsNil() && containsMutableValue(value.Elem())
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return !value.IsNil()
	case reflect.Array:
		for index := range value.Len() {
			if containsMutableValue(value.Index(index)) {
				return true
			}
		}
	case reflect.Struct:
		for index := range value.NumField() {
			if containsMutableValue(value.Field(index)) {
				return true
			}
		}
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return false
	}

	return false
}

func newSnapshotCloneError(path string, typeOf reflect.Type) *SnapshotCloneError {
	return &SnapshotCloneError{Path: path, Type: typeOf.String()}
}

func cloneAssistantMessage(message AssistantMessage) (AssistantMessage, error) {
	return newSnapshotCloner().cloneAssistantMessage(message)
}

func (cloner *snapshotCloner) cloneAssistantMessage(
	message AssistantMessage,
) (AssistantMessage, error) {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]AssistantContent, len(message.Content))
		for index, content := range message.Content {
			clonedContent, err := cloner.cloneAssistantContent(content)
			if err != nil {
				return AssistantMessage{}, err
			}
			cloned.Content[index] = clonedContent
		}
	}
	cloned.Usage.CacheWrite1HTokens = cloneInt64(message.Usage.CacheWrite1HTokens)
	cloned.Usage.ReasoningTokens = cloneInt64(message.Usage.ReasoningTokens)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneAssistantContent(
	content AssistantContent,
) (AssistantContent, error) {
	switch value := content.(type) {
	case TextContent:
		return value, nil
	case *TextContent:
		if value == nil {
			return value, nil
		}

		cloned := *value

		return &cloned, nil
	case ThinkingContent:
		return value, nil
	case *ThinkingContent:
		if value == nil {
			return value, nil
		}

		cloned := *value

		return &cloned, nil
	case ToolCall:
		return cloner.cloneToolCall(value)
	case *ToolCall:
		if value == nil {
			return value, nil
		}

		cloned, err := cloner.cloneToolCall(*value)
		if err != nil {
			return nil, err
		}

		return &cloned, nil
	}

	return content, nil
}

func (cloner *snapshotCloner) cloneToolCall(call ToolCall) (ToolCall, error) {
	if call.Arguments == nil {
		return call, nil
	}
	clonedArguments, err := cloner.cloneValue(
		reflect.ValueOf(call.Arguments),
		"ToolCall.Arguments",
	)
	if err != nil {
		return ToolCall{}, err
	}
	arguments, ok := clonedArguments.Interface().(map[string]any)
	if !ok {
		return ToolCall{}, &SnapshotCloneError{
			Path: "ToolCall.Arguments",
			Type: clonedArguments.Type().String(),
		}
	}

	cloned := call
	cloned.Arguments = arguments

	return cloned, nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func (cloner *snapshotCloner) cloneAgentMessage(message AgentMessage) (AgentMessage, error) {
	if message == nil {
		return nil, &SnapshotCloneError{Path: "AgentMessage", Type: "<nil>"}
	}
	messageValue := reflect.ValueOf(message)
	if messageValue.Kind() == reflect.Pointer {
		cloned, err := cloner.cloneValue(messageValue, "AgentMessage")
		if err != nil {
			return nil, err
		}
		clonedMessage, ok := cloned.Interface().(AgentMessage)
		if !ok {
			return nil, newSnapshotCloneError("AgentMessage", cloned.Type())
		}
		return clonedMessage, nil
	}

	switch value := message.(type) {
	case UserMessage:
		return cloner.cloneUserMessage(value)
	case AssistantMessage:
		return cloner.cloneAssistantMessage(value)
	case ToolResultMessage:
		return cloner.cloneToolResultMessage(value)
	case CustomMessage:
		if value.Payload != nil {
			payload, err := cloner.cloneAny(value.Payload, "CustomMessage.Payload")
			if err != nil {
				return nil, err
			}
			value.Payload = payload
		}
		return value, nil
	}

	return message, nil
}

func (cloner *snapshotCloner) cloneUserMessage(message UserMessage) (UserMessage, error) {
	cloned := message
	cloned.Content = make([]UserContent, len(message.Content))
	for index, content := range message.Content {
		switch value := content.(type) {
		case *TextContent:
			if value != nil {
				copied := *value
				cloned.Content[index] = &copied
			}
		case *ImageContent:
			if value != nil {
				copied := *value
				cloned.Content[index] = &copied
			}
		default:
			cloned.Content[index] = content
		}
	}
	return cloned, nil
}

func (cloner *snapshotCloner) cloneToolResultMessage(message ToolResultMessage) (ToolResultMessage, error) {
	cloned := message
	cloned.Content = cloneToolResultContent(message.Content)
	if message.Details != nil {
		details, err := cloner.cloneAny(message.Details, "ToolResultMessage.Details")
		if err != nil {
			return ToolResultMessage{}, err
		}
		cloned.Details = details
	}
	cloned.Usage = cloneUsage(message.Usage)
	cloned.AddedToolNames = append([]string(nil), message.AddedToolNames...)
	return cloned, nil
}

func (cloner *snapshotCloner) cloneToolResult(result ToolResult) (ToolResult, error) {
	cloned := result
	cloned.Content = cloneToolResultContent(result.Content)
	if result.Details != nil {
		details, err := cloner.cloneAny(result.Details, "ToolResult.Details")
		if err != nil {
			return ToolResult{}, err
		}
		cloned.Details = details
	}
	cloned.Usage = cloneUsage(result.Usage)
	cloned.AddedToolNames = append([]string(nil), result.AddedToolNames...)
	return cloned, nil
}

func cloneToolResultContent(content []ToolResultContent) []ToolResultContent {
	cloned := make([]ToolResultContent, len(content))
	for index, item := range content {
		switch value := item.(type) {
		case *TextContent:
			if value != nil {
				copied := *value
				cloned[index] = &copied
			}
		case *ImageContent:
			if value != nil {
				copied := *value
				cloned[index] = &copied
			}
		default:
			cloned[index] = item
		}
	}
	return cloned
}

func (cloner *snapshotCloner) cloneAny(value any, path string) (any, error) {
	cloned, err := cloner.cloneValue(reflect.ValueOf(value), path)
	if err != nil {
		return nil, err
	}
	return cloned.Interface(), nil
}

func cloneUsage(usage *Usage) *Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	cloned.CacheWrite1HTokens = cloneInt64(usage.CacheWrite1HTokens)
	cloned.ReasoningTokens = cloneInt64(usage.ReasoningTokens)
	return &cloned
}
