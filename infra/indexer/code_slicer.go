package indexer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/david22573/codepicker/domain/context"
)

type CodeSlicer struct {
	fset *token.FileSet
}

func NewCodeSlicer() *CodeSlicer {
	return &CodeSlicer{
		fset: token.NewFileSet(),
	}
}

// SliceFile parses a Go file and breaks it into semantic CodeSlices.
// ENHANCEMENT: Now associates methods with their receiver types.
func (s *CodeSlicer) SliceFile(filePath string) ([]context.CodeSlice, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	node, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AST: %w", err)
	}

	var slices []context.CodeSlice
	fileHash := computeHash(content)

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// 1. Extract Name
			name := d.Name.Name

			// 2. Identify Receiver (Method vs Function)
			var symbols []string
			sliceType := context.SliceTypeFunction

			if d.Recv != nil && len(d.Recv.List) > 0 {
				// It's a method. Try to get the receiver type name.
				recvType := ""
				switch t := d.Recv.List[0].Type.(type) {
				case *ast.StarExpr: // e.g. *MyStruct
					if ident, ok := t.X.(*ast.Ident); ok {
						recvType = ident.Name
					}
				case *ast.Ident: // e.g. MyStruct
					recvType = t.Name
				}

				if recvType != "" {
					// Symbol format: "MyStruct.MethodName"
					symbols = append(symbols, recvType, fmt.Sprintf("%s.%s", recvType, name))
					sliceType = context.SliceTypeFunction // Domain doesn't have "Method" type yet, keep as Function
				} else {
					symbols = append(symbols, name)
				}
			} else {
				symbols = append(symbols, name)
			}

			slices = append(slices, s.createSlice(filePath, d, sliceType, symbols, fileHash))

		case *ast.GenDecl:
			// 3. Extract Types (Structs/Interfaces)
			for _, spec := range d.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					typeName := typeSpec.Name.Name
					sType := s.getSliceType(typeSpec)

					// Just the type name as symbol
					symbols := []string{typeName}

					slices = append(slices, s.createSlice(filePath, d, sType, symbols, fileHash))
				}
			}
		}
	}

	return slices, nil
}

func (s *CodeSlicer) createSlice(path string, node ast.Node, sType context.SliceType, symbols []string, fileHash string) context.CodeSlice {
	start := s.fset.Position(node.Pos()).Line
	end := s.fset.Position(node.End()).Line

	var buf bytes.Buffer
	format.Node(&buf, s.fset, node)

	// Primary symbol is usually the last one (e.g. "Struct.Method") or just the name
	primaryName := symbols[0]
	if len(symbols) > 1 {
		primaryName = symbols[len(symbols)-1]
	}
	// Sanitize ID
	safeName := strings.ReplaceAll(primaryName, "*", "")

	return context.CodeSlice{
		ID:        fmt.Sprintf("%s-%s-%d", path, safeName, start),
		FilePath:  path,
		StartLine: start,
		EndLine:   end,
		Content:   buf.String(),
		Language:  "go",
		SliceType: sType,
		Symbols:   symbols,
		Hash:      fileHash,
	}
}

func (s *CodeSlicer) getSliceType(spec *ast.TypeSpec) context.SliceType {
	switch spec.Type.(type) {
	case *ast.StructType:
		return context.SliceTypeStruct
	case *ast.InterfaceType:
		return context.SliceTypeInterface
	default:
		return context.SliceTypeBlock
	}
}

func computeHash(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
