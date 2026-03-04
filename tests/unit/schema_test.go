package unit

import (
	"encoding/json"
	"testing"

	"github.com/david22573/codepicker/infra/llm"
)

func TestGenerateSchema(t *testing.T) {
	type WriteFileInput struct {
		Path    string `json:"path" desc:"The file path to write to"`
		Content string `json:"content" desc:"The complete file content to write"`
		Lines   int    `json:"lines,omitempty"` // Ensure omitempty parsing works
		Ignored string `json:"-"`               // Should not appear in schema
	}

	schema := llm.GenerateSchema(WriteFileInput{})

	// 1. Check top-level type
	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", schema["type"])
	}

	// 2. Check required array
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected required to be []string")
	}
	if len(required) != 3 { // path, content, lines
		t.Errorf("expected 3 required fields, got %d", len(required))
	}

	// 3. Check properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties to be map[string]interface{}")
	}

	// Check path field
	pathProp, ok := properties["path"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected path property to exist")
	}
	if pathProp["type"] != "string" {
		t.Errorf("expected path type 'string', got '%v'", pathProp["type"])
	}
	if pathProp["description"] != "The file path to write to" {
		t.Errorf("expected description to match struct tag")
	}

	// Check lines field (number type mapping)
	linesProp, ok := properties["lines"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected lines property to exist")
	}
	if linesProp["type"] != "integer" {
		t.Errorf("expected lines type 'integer', got '%v'", linesProp["type"])
	}

	// Check ignored field
	if _, exists := properties["Ignored"]; exists {
		t.Errorf("expected Ignored field to be skipped due to `json:\"-\"` tag")
	}

	// Validate it marshals correctly to JSON
	_, err := json.Marshal(schema)
	if err != nil {
		t.Errorf("failed to marshal generated schema to JSON: %v", err)
	}
}

func TestGenerateToolDefinition(t *testing.T) {
	type DummyInput struct {
		ID string `json:"id"`
	}

	def := llm.GenerateToolDefinition("dummy_tool", "A tool for testing", DummyInput{})

	if def.Type != "function" {
		t.Errorf("expected Type 'function', got '%s'", def.Type)
	}
	if def.Function.Name != "dummy_tool" {
		t.Errorf("expected Name 'dummy_tool', got '%s'", def.Function.Name)
	}
	if def.Function.Description != "A tool for testing" {
		t.Errorf("expected Description match, got '%s'", def.Function.Description)
	}
}
