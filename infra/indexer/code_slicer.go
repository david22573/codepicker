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

// SliceFile parses a Go file and breaks it into semantic CodeSlices
func (s *CodeSlicer) SliceFile(filePath string) ([]context.CodeSlice, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 1. Parse the file into an AST
	node, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AST: %w", err)
	}

	var slices []context.CodeSlice
	fileHash := computeHash(content)

	// 2. Walk the AST to find top-level declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Extract Function/Method
			slices = append(slices, s.createSlice(filePath, d, context.SliceTypeFunction, d.Name.Name, content, fileHash))

		case *ast.GenDecl:
			// Extract Types (Structs/Interfaces)
			for _, spec := range d.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					slices = append(slices, s.createSlice(filePath, d, s.getSliceType(typeSpec), typeSpec.Name.Name, content, fileHash))
				}
			}
		}
	}

	return slices, nil
}

// createSlice transforms an AST node into our domain CodeSlice model
func (s *CodeSlicer) createSlice(path string, node ast.Node, sType context.SliceType, name string, fullContent []byte, fileHash string) context.CodeSlice {
	start := s.fset.Position(node.Pos()).Line
	end := s.fset.Position(node.End()).Line

	// Extract the actual source code for this node
	var buf bytes.Buffer
	format.Node(&buf, s.fset, node)

	return context.CodeSlice{
		ID:        fmt.Sprintf("%s-%s-%d", path, name, start),
		FilePath:  path,
		StartLine: start,
		EndLine:   end,
		Content:   buf.String(),
		Language:  "go",
		SliceType: sType,
		Symbols:   []string{name},
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
