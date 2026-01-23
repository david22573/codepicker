package code

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SkeletonOptions struct {
	KeepDocComments bool // If true, keeps function/type documentation
	SkipTests       bool // If true, ignores _test.go files
}

// GenerateSkeleton walks a directory or processes a single file path from disk.
func GenerateSkeleton(path string, opts SkeletonOptions) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		// Read file content and pass to Skeletonize
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		result, err := Skeletonize(path, content, opts.KeepDocComments)
		if err != nil {
			return "", err
		}
		return string(result), nil
	}

	var output strings.Builder

	err = filepath.WalkDir(path, func(walkPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			return filepath.SkipDir
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
			if opts.SkipTests && strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			// Read file content
			content, err := os.ReadFile(walkPath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", walkPath, err)
			}

			skeleton, err := Skeletonize(walkPath, content, opts.KeepDocComments)
			if err != nil {
				return fmt.Errorf("failed to process %s: %w", walkPath, err)
			}

			output.WriteString(fmt.Sprintf("\n## File: %s\n```go\n%s\n```\n", walkPath, string(skeleton)))
		}

		return nil
	})

	return output.String(), err
}

// Skeletonize takes raw source code and returns a stripped-down version (signatures only).
// This fixes the missing function reference in planner.go and skeleton_tool.go.
func Skeletonize(filename string, src []byte, keepDocs bool) ([]byte, error) {
	fset := token.NewFileSet()

	mode := parser.ParseComments
	if !keepDocs {
		mode = 0
	}

	// Parse the source from the byte slice
	node, err := parser.ParseFile(fset, filename, src, mode)
	if err != nil {
		return nil, err
	}

	// Remove function bodies
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Body != nil {
				fn.Body.List = nil // Clear statements
			}
		}
		return true
	})

	// Remove comments if requested
	if !keepDocs {
		node.Comments = nil
		node.Doc = nil
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return nil, err
	}

	// Cleanup whitespace
	cleaned := cleanupOutput(buf.String())
	return []byte(cleaned), nil
}

func cleanupOutput(src string) string {
	lines := strings.Split(src, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
