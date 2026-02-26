package tools

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/validation"
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

	if err := validation.DecodeStrict(args, &input); err != nil {
		return "", errors.NewValidation("tool.search_definition", "invalid JSON arguments: "+err.Error())
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
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := fs.SafeReadFile(ctx, path)
		if err != nil {
			return nil
		}

		node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return nil
		}

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

func getLineFromContent(content string, lineNum int) string {
	lines := strings.Split(content, "\n")
	if lineNum > 0 && lineNum <= len(lines) {
		return strings.TrimSpace(lines[lineNum-1])
	}
	return ""
}