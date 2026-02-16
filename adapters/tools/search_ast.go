package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
)

type DefinitionSearchTool struct {
	projectRoot string
}

func NewDefinitionSearchTool(root string) *DefinitionSearchTool {
	return &DefinitionSearchTool{projectRoot: root}
}

func (t *DefinitionSearchTool) Name() string { return "search_definition" }
func (t *DefinitionSearchTool) Description() string {
	return `Find the definition of a specific Go symbol (function, struct, interface).
Input JSON: {"name": "SymbolName"}`
}

func (t *DefinitionSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.search_definition", "invalid JSON arguments")
	}

	if input.Name == "" {
		return "", errors.NewValidation("tool.search_definition", "symbol name is required")
	}

	fset := token.NewFileSet()
	var results strings.Builder
	foundCount := 0

	err := filepath.Walk(t.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip non-Go files and hidden directories
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// FIX: Use SafeReadFile to get content before parsing to avoid reading huge files
		content, err := fs.SafeReadFile(ctx, path)
		if err != nil {
			// Skip files that are unsafe to read
			return nil
		}

		// Parse the file using the safe content
		node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return nil
		}

		// Inspect the AST for the symbol
		ast.Inspect(node, func(n ast.Node) bool {
			var matchType string
			var matchName string
			var matchPos token.Pos

			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Name.Name == input.Name {
					matchName = x.Name.Name
					matchPos = x.Pos()
					if x.Recv != nil {
						matchType = "Method"
					} else {
						matchType = "Function"
					}
				}
			case *ast.TypeSpec:
				if x.Name.Name == input.Name {
					matchName = x.Name.Name
					matchPos = x.Pos()
					matchType = "Type"
				}
			}

			if matchName != "" {
				relPath, _ := filepath.Rel(t.projectRoot, path)
				position := fset.Position(matchPos)

				// Capture the exact line of code
				// FIX: Reuse the content we already safely read
				lineContent := getLineFromContent(string(content), position.Line)

				results.WriteString(fmt.Sprintf("[%s] %s:%d\n%s\n\n", matchType, relPath, position.Line, lineContent))
				foundCount++
			}
			return true
		})

		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.search_definition", "walk failed", err)
	}

	if foundCount == 0 {
		return fmt.Sprintf("No definitions found for symbol '%s'.", input.Name), nil
	}

	return results.String(), nil
}

// Helper to extract line from string content (memory safe)
func getLineFromContent(content string, lineNum int) string {
	lines := strings.Split(content, "\n")
	if lineNum > 0 && lineNum <= len(lines) {
		return strings.TrimSpace(lines[lineNum-1])
	}
	return ""
}
