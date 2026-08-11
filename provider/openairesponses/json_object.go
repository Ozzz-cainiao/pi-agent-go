package openairesponses

import (
	"encoding/json"
	"fmt"
)

func decodeJSONObject(raw, operation string) (map[string]any, error) {
	if raw == "" {
		raw = "{}"
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return nil, &ProtocolError{Operation: operation, Cause: err}
	}
	if object == nil {
		return nil, &ProtocolError{Operation: operation, Cause: fmt.Errorf("arguments must be a JSON object")}
	}
	return object, nil
}

func cloneJSONObject(object map[string]any) map[string]any {
	if object == nil {
		return nil
	}
	cloned := make(map[string]any, len(object))
	for key, value := range object {
		cloned[key] = cloneJSONValue(value)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return typed
	}
}
