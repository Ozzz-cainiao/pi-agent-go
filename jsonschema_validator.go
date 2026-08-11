package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const toolArgumentsSchemaURL = "tool-arguments-schema.json"

// JSONSchemaArgumentValidator 使用 JSON Schema Draft 2020-12 校验参数。
type JSONSchemaArgumentValidator struct{}

func (JSONSchemaArgumentValidator) Validate(
	ctx context.Context,
	definition ToolDefinition,
	arguments map[string]any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(definition.Parameters) == 0 {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	var document any
	if err := json.Unmarshal(definition.Parameters, &document); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	if err := compiler.AddResource(toolArgumentsSchemaURL, document); err != nil {
		return fmt.Errorf("load schema: %w", err)
	}
	schema, err := compiler.Compile(toolArgumentsSchemaURL)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := schema.Validate(arguments); err != nil {
		return fmt.Errorf("schema path %s: %w", schemaPath(err), err)
	}
	return nil
}

func schemaPath(err error) string {
	var validationError *jsonschema.ValidationError
	if !errors.As(err, &validationError) {
		return "/"
	}
	for _, cause := range validationError.Causes {
		if path := schemaPath(cause); path != "/" {
			return path
		}
	}
	if validationError.ErrorKind != nil {
		keywords := validationError.ErrorKind.KeywordPath()
		if len(keywords) > 0 {
			return joinSchemaPath(schemaFragment(validationError.SchemaURL), keywords)
		}
	}
	return "/"
}

func schemaFragment(schemaURL string) []string {
	parsed, err := url.Parse(schemaURL)
	if err != nil || parsed.Fragment == "" {
		return nil
	}
	return strings.Split(strings.Trim(parsed.Fragment, "/"), "/")
}

func joinSchemaPath(fragment, keywords []string) string {
	parts := append(append([]string(nil), fragment...), keywords...)
	return "/" + strings.Join(parts, "/")
}
