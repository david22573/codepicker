package minifier

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

type GoMinifier struct{}

func (m *GoMinifier) Minify(content []byte) []byte {
	fset := token.NewFileSet()

	// Parse the file
	f, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		// If AST parsing fails (syntax error), fallback to line-based minification
		fallback := &GenericMinifier{}
		return fallback.Minify(content)
	}

	// Remove all comments
	f.Comments = nil

	// Remove doc strings from AST nodes
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.File:
			x.Doc = nil
		case *ast.GenDecl:
			x.Doc = nil
		case *ast.FuncDecl:
			x.Doc = nil
		case *ast.TypeSpec:
			x.Doc = nil
		case *ast.Field:
			x.Doc = nil
		}
		return true
	})

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return content
	}

	return buf.Bytes()
}

