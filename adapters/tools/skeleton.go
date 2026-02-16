package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
)

type SkeletonTool struct {
	projectRoot string
}

func NewSkeletonTool(root string) *SkeletonTool {
	return &SkeletonTool{projectRoot: root}
}

func (t *SkeletonTool) Name() string { return "read_skeleton" }
func (t *SkeletonTool) Description() string {
	return `Read the structure (skeleton) of a Go file or directory without implementation details.
Input JSON: {"path": "string"}`
}

func (t *SkeletonTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.read_skeleton", "invalid JSON arguments")
	}

	if input.Path == "" {
		input.Path = "."
	}

	targetPath := filepath.Join(t.projectRoot, input.Path)
	info, err := os.Stat(targetPath)
	if err != nil {
		return "", errors.NewValidation("tool.read_skeleton", "path not found")
	}

	var results strings.Builder
	fset := token.NewFileSet()

	// Handler for a single file
	processFile := func(currPath string) error {
		if !strings.HasSuffix(currPath, ".go") {
			return nil
		}

		content, err := fs.SafeReadFile(ctx, currPath)
		if err != nil {
			// Skip files that are too large or binary
			return nil
		}

		// Parse using the safely read content
		node, err := parser.ParseFile(fset, currPath, content, parser.ParseComments)
		if err != nil {
			return nil // Skip unparsable
		}

		// Prune the AST: Remove function bodies
		ast.Inspect(node, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				fn.Body = nil // Remove implementation
			}
			return true
		})

		// Render the skeleton back to source code
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, node); err != nil {
			return err
		}

		relPath, _ := filepath.Rel(t.projectRoot, currPath)
		results.WriteString(fmt.Sprintf("## Skeleton: %s\n```go\n%s\n```\n\n", relPath, buf.String()))
		return nil
	}

	// Logic for Dir vs File
	if !info.IsDir() {
		if err := processFile(targetPath); err != nil {
			return "", err
		}
	} else {
		// Walk the directory
		files, err := os.ReadDir(targetPath)
		if err != nil {
			return "", err
		}
		for _, f := range files {
			if !f.IsDir() {
				_ = processFile(filepath.Join(targetPath, f.Name()))
			}
		}
	}

	if results.Len() == 0 {
		return "No Go files found to skeletonize.", nil
	}

	return results.String(), nil
}
