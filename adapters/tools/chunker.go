package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// CodeChunk represents a semantic unit of code.
type CodeChunk struct {
	Name    string
	Content string
	Start   int
	End     int
}

// ChunkGoFile parses a Go source file and extracts high-level declarations as chunks.
func ChunkGoFile(path string) ([]CodeChunk, error) {
	fset := token.NewFileSet()
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse file with comments preserved
	node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var chunks []CodeChunk
	for _, decl := range node.Decls {
		var name string
		var start, end token.Pos

		switch d := decl.(type) {
		case *ast.FuncDecl:
			name = d.Name.Name
			start = d.Pos()
			end = d.End()
		case *ast.GenDecl:
			// Handle types, structs, and interfaces
			if len(d.Specs) > 0 {
				if ts, ok := d.Specs[0].(*ast.TypeSpec); ok {
					name = ts.Name.Name
					start = d.Pos()
					end = d.End()
				}
			}
		}

		if name != "" {
			startOff := fset.Position(start).Offset
			endOff := fset.Position(end).Offset

			chunks = append(chunks, CodeChunk{
				Name:    name,
				Content: string(content[startOff:endOff]),
				Start:   fset.Position(start).Line,
				End:     fset.Position(end).Line,
			})
		}
	}

	// If no chunks were found (e.g. only imports), return the whole file as one chunk
	if len(chunks) == 0 {
		chunks = append(chunks, CodeChunk{
			Name:    "full_file",
			Content: string(content),
			Start:   1,
			End:     strings.Count(string(content), "\n") + 1,
		})
	}

	return chunks, nil
}
