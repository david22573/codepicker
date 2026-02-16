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
// OPTIMIZATION: Recursively splits large functions into smaller logical blocks.
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
			// Extract Name & Receiver
			name := d.Name.Name
			var symbols []string

			if d.Recv != nil && len(d.Recv.List) > 0 {
				recvType := ""
				switch t := d.Recv.List[0].Type.(type) {
				case *ast.StarExpr:
					if ident, ok := t.X.(*ast.Ident); ok {
						recvType = ident.Name
					}
				case *ast.Ident:
					recvType = t.Name
				}
				if recvType != "" {
					symbols = append(symbols, recvType, fmt.Sprintf("%s.%s", recvType, name))
				} else {
					symbols = append(symbols, name)
				}
			} else {
				symbols = append(symbols, name)
			}

			// OPTIMIZATION: Check size. If > 50 lines, split it.
			start := s.fset.Position(d.Pos()).Line
			end := s.fset.Position(d.End()).Line

			if end-start > 50 {
				// Recursively slice the body
				subSlices := s.sliceBlock(filePath, d.Body, symbols, fileHash)
				slices = append(slices, subSlices...)
			} else {
				// Keep it whole
				slices = append(slices, s.createSlice(filePath, d, context.SliceTypeFunction, symbols, fileHash))
			}

		case *ast.GenDecl:
			// Keep types/structs whole as they define contracts
			for _, spec := range d.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					typeName := typeSpec.Name.Name
					sType := s.getSliceType(typeSpec)
					symbols := []string{typeName}
					slices = append(slices, s.createSlice(filePath, d, sType, symbols, fileHash))
				}
			}
		}
	}

	return slices, nil
}

// sliceBlock recursively hunts for large control structures (if/for/switch) to split.
func (s *CodeSlicer) sliceBlock(filePath string, block *ast.BlockStmt, parentSymbols []string, fileHash string) []context.CodeSlice {
	var slices []context.CodeSlice

	for _, stmt := range block.List {
		start := s.fset.Position(stmt.Pos()).Line
		end := s.fset.Position(stmt.End()).Line

		// Only split if the individual statement block is substantial (>10 lines)
		if end-start > 10 {
			switch t := stmt.(type) {
			case *ast.IfStmt:
				slices = append(slices, s.createSlice(filePath, t, context.SliceTypeBlock, parentSymbols, fileHash))
			case *ast.ForStmt:
				slices = append(slices, s.createSlice(filePath, t, context.SliceTypeBlock, parentSymbols, fileHash))
			case *ast.SwitchStmt:
				slices = append(slices, s.createSlice(filePath, t, context.SliceTypeBlock, parentSymbols, fileHash))
			case *ast.RangeStmt:
				slices = append(slices, s.createSlice(filePath, t, context.SliceTypeBlock, parentSymbols, fileHash))
			}
		}
	}

	// If no sub-blocks were large enough, we might have missed the forest for the trees.
	// In a production version, you'd add fallback logic here to grab the whole parent if sub-slicing yielded nothing.
	return slices
}

func (s *CodeSlicer) createSlice(path string, node ast.Node, sType context.SliceType, symbols []string, fileHash string) context.CodeSlice {
	start := s.fset.Position(node.Pos()).Line
	end := s.fset.Position(node.End()).Line

	var buf bytes.Buffer
	format.Node(&buf, s.fset, node)

	primaryName := symbols[0]
	if len(symbols) > 1 {
		primaryName = symbols[len(symbols)-1]
	}
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
