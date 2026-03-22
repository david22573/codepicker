package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func parseGo(content []byte, fm *FileMap) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fm.Path, content, parser.AllErrors)
	if err != nil {
		return
	}

	fm.Package = node.Name.Name

	for _, imp := range node.Imports {
		if imp.Path != nil {
			fm.Imports = append(fm.Imports, imp.Path.Value)
		}
	}

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				fm.Symbols = append(fm.Symbols, Symbol{
					Name:      d.Name.Name,
					Kind:      "func",
					Signature: extractGoNode(fset, d, content),
					Lines:     fset.Position(d.End()).Line - fset.Position(d.Pos()).Line,
				})
			}
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
						fm.Symbols = append(fm.Symbols, Symbol{
							Name:      ts.Name.Name,
							Kind:      "type",
							Signature: "type " + ts.Name.Name,
							Lines:     fset.Position(ts.End()).Line - fset.Position(ts.Pos()).Line,
						})
					}
				}
			}
		}
	}
}

func extractGoNode(fset *token.FileSet, node ast.Node, content []byte) string {
	start := fset.Position(node.Pos()).Offset
	end := fset.Position(node.End()).Offset
	if start < 0 || end > len(content) || start > end {
		return ""
	}
	text := string(content[start:end])
	idx := strings.Index(text, "{")
	if idx > 0 {
		return strings.TrimSpace(text[:idx])
	}
	return strings.Split(text, "\n")[0]
}
