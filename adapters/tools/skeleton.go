package tools

import (
	"bytes"
	"context"
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
	"github.com/david22573/codepicker/infra/validation"
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
	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", errors.NewValidation("tool.read_skeleton", "invalid JSON arguments: "+err.Error())
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

	processFile := func(currPath string) error {
		if !strings.HasSuffix(currPath, ".go") {
			return nil
		}

		content, err := fs.SafeReadFile(ctx, currPath)
		if err != nil {
			return nil
		}

		node, err := parser.ParseFile(fset, currPath, content, parser.ParseComments)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				fn.Body = nil
			}
			return true
		})

		var buf bytes.Buffer
		if err := format.Node(&buf, fset, node); err != nil {
			return err
		}

		relPath, _ := filepath.Rel(t.projectRoot, currPath)
		results.WriteString(fmt.Sprintf("## Skeleton: %s\n```go\n%s\n```\n\n", relPath, buf.String()))
		return nil
	}

	if !info.IsDir() {
		if err := processFile(targetPath); err != nil {
			return "", err
		}
	} else {
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
