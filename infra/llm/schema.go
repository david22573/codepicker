package llm

import (
	"reflect"
	"strings"
)

// GenerateToolDefinition creates a complete ToolDefinition from a struct instance.
// It uses reflection to build the JSON schema for the parameters.
func GenerateToolDefinition(name, description string, inputStruct interface{}) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  GenerateSchema(inputStruct),
		},
	}
}

// GenerateSchema uses reflection to generate a JSON Schema from a Go struct.
func GenerateSchema(v interface{}) map[string]interface{} {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	properties := make(map[string]interface{})
	required := make([]string, 0)

	if t.Kind() != reflect.Struct {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")

		// Skip fields without JSON tags or ignored fields
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Handle "name,omitempty"
		parts := strings.Split(jsonTag, ",")
		fieldName := parts[0]

		// Determine JSON type
		jsonType := "string"
		switch field.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			jsonType = "integer"
		case reflect.Float32, reflect.Float64:
			jsonType = "number"
		case reflect.Bool:
			jsonType = "boolean"
		case reflect.Slice, reflect.Array:
			jsonType = "array"
		}

		// Build property definition
		propDef := map[string]interface{}{
			"type": jsonType,
		}

		// Add description if available via a custom tag (optional enhancement)
		if desc := field.Tag.Get("desc"); desc != "" {
			propDef["description"] = desc
		}

		properties[fieldName] = propDef
		required = append(required, fieldName)
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}
