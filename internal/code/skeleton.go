package code

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// Skeletonize parses Go source code and removes function bodies,
// leaving only signatures, types, and global variables.
func Skeletonize(filename string, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing failed: %w", err)
	}

	// Remove file-level comments to save space, but keep package docs if strictly necessary
	// generally agents don't need the copyright headers.
	file.Doc = nil

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			// We keep the function signature/declaration but strip the body.
			if x.Body != nil {
				// Create a comment to indicate where code was removed
				// Note: AST manipulation for comments inside a block can be tricky with formatting.
				// The cleanest way is to simply empty the statement list.
				x.Body.List = nil
			}
		}
		return true
	})

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("formatting failed: %w", err)
	}

	return buf.Bytes(), nil
}
