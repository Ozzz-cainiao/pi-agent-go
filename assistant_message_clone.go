package agent

import "reflect"

func cloneAssistantMessage(message AssistantMessage) AssistantMessage {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]AssistantContent, len(message.Content))
		for index, content := range message.Content {
			cloned.Content[index] = cloneAssistantContent(content)
		}
	}
	cloned.Usage.CacheWrite1HTokens = cloneInt64(message.Usage.CacheWrite1HTokens)
	cloned.Usage.ReasoningTokens = cloneInt64(message.Usage.ReasoningTokens)

	return cloned
}

func cloneAssistantContent(content AssistantContent) AssistantContent {
	switch value := content.(type) {
	case TextContent:
		return value
	case *TextContent:
		if value == nil {
			return value
		}

		cloned := *value

		return &cloned
	case ThinkingContent:
		return value
	case *ThinkingContent:
		if value == nil {
			return value
		}

		cloned := *value

		return &cloned
	case ToolCall:
		return cloneToolCall(value)
	case *ToolCall:
		if value == nil {
			return value
		}

		cloned := cloneToolCall(*value)

		return &cloned
	}

	return content
}

func cloneToolCall(call ToolCall) ToolCall {
	cloned := call
	cloned.Arguments = cloneArguments(call.Arguments)

	return cloned
}

func cloneArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}

	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = cloneArgumentValue(value)
	}

	return cloned
}

func cloneArgumentValue(value any) any {
	cloned := cloneMutableValue(reflect.ValueOf(value))
	if !cloned.IsValid() {
		return nil
	}

	return cloned.Interface()
}

func cloneMutableValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		cloned := reflect.New(value.Type()).Elem()
		cloned.Set(cloneMutableValue(value.Elem()))

		return cloned
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			cloned.SetMapIndex(iterator.Key(), cloneMutableValue(iterator.Value()))
		}

		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := range value.Len() {
			cloned.Index(index).Set(cloneMutableValue(value.Index(index)))
		}

		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for index := range value.Len() {
			cloned.Index(index).Set(cloneMutableValue(value.Index(index)))
		}

		return cloned
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneMutableValue(value.Elem()))

		return cloned
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
		reflect.Chan,
		reflect.Func,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		return value
	}

	return value
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}
