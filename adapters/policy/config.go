package policy

import (
	"encoding/json"
	"os"
)

// PolicySchema defines the structure of policy.json
type PolicySchema struct {
	AllowedGlobs      []string `json:"allowed_globs"`
	AllowedIssueTypes []string `json:"allowed_issue_types"`
	ForbiddenKeywords []string `json:"forbidden_keywords"`
}

// DefaultPolicy returns a safe baseline if no file exists
func DefaultPolicy() PolicySchema {
	return PolicySchema{
		// Default to allowing everything in src, but protecting sensitive dirs
		AllowedGlobs: []string{
			"**/*.go",
			"**/*.md",
			"Makefile",
		},
		ForbiddenKeywords: []string{
			"rm -rf",
			"drop table",
		},
	}
}

// LoadPolicy loads policy.json from the path, or returns default if missing
func LoadPolicy(path string) (*PolicySchema, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		def := DefaultPolicy()
		return &def, nil
	}
	if err != nil {
		return nil, err
	}

	var schema PolicySchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}
