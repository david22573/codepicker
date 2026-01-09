package writer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
)

// OutputStrategy defines how we handle a found file
type OutputStrategy interface {
	Init() error
	Write(absPath, relPath string) error
	Close() error
	ShouldSkip(path string) bool
}

// --- Strategy 1: Concat (Single File) ---

type ConcatStrategy struct {
	OutputPath string
	file       *os.File
}

func NewConcatStrategy(path string) *ConcatStrategy {
	return &ConcatStrategy{OutputPath: path}
}

func (c *ConcatStrategy) Init() error {
	if err := os.MkdirAll(filepath.Dir(c.OutputPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(c.OutputPath)
	if err != nil {
		return err
	}
	c.file = f
	fmt.Fprintf(c.file, "CODEPICKER CONTEXT DUMP\n=========================\n\n")
	return nil
}

func (c *ConcatStrategy) ShouldSkip(path string) bool {
	return path == c.OutputPath
}

func (c *ConcatStrategy) Write(absPath, relPath string) error {
	fmt.Printf("   Picked: %s\n", relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	if !utf8.Valid(content) {
		return nil
	}
	fmt.Fprintf(c.file, "FILE PATH: %s\n------------------------------------------------------------------------------\n", relPath)
	c.file.Write(content)
	fmt.Fprintf(c.file, "\n\n")
	return nil
}

func (c *ConcatStrategy) Close() error {
	return c.file.Close()
}

// --- Strategy 2: Copy (Directory Mirror) ---

type CopyStrategy struct {
	OutputDir string
}

func NewCopyStrategy(dir string) *CopyStrategy {
	return &CopyStrategy{OutputDir: dir}
}

func (c *CopyStrategy) Init() error {
	return os.MkdirAll(c.OutputDir, 0755)
}

func (c *CopyStrategy) ShouldSkip(path string) bool {
	return filepath.HasPrefix(path, c.OutputDir)
}

func (c *CopyStrategy) Write(absPath, relPath string) error {
	fmt.Printf("   Picked: %s\n", relPath)
	targetPath := filepath.Join(c.OutputDir, relPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	src, _ := os.Open(absPath)
	defer src.Close()
	dst, _ := os.Create(targetPath)
	defer dst.Close()
	io.Copy(dst, src)
	return nil
}

func (c *CopyStrategy) Close() error { return nil }

// --- Strategy 3: Tree (Visual Map) ---

type TreeOptions struct {
	CopyToClipboard bool
	OutPath         string
}

type TreeStrategy struct {
	opts   TreeOptions
	buffer *bytes.Buffer
}

func NewTreeStrategy(opts TreeOptions) *TreeStrategy {
	return &TreeStrategy{
		opts:   opts,
		buffer: &bytes.Buffer{},
	}
}

func (t *TreeStrategy) Init() error {
	header := "PROJECT STRUCTURE:\n.\n"
	fmt.Print(header)            // Screen
	t.buffer.WriteString(header) // Buffer
	return nil
}

func (t *TreeStrategy) ShouldSkip(path string) bool {
	if t.opts.OutPath != "" {
		return strings.HasSuffix(path, t.opts.OutPath)
	}
	return false
}

func (t *TreeStrategy) Write(absPath, relPath string) error {
	parts := strings.Split(relPath, string(os.PathSeparator))
	depth := len(parts) - 1
	filename := parts[len(parts)-1]

	indent := ""
	for i := 0; i < depth; i++ {
		indent += "│   "
	}

	line := fmt.Sprintf("%s├── %s\n", indent, filename)
	fmt.Print(line)            // Screen
	t.buffer.WriteString(line) // Buffer
	return nil
}

func (t *TreeStrategy) Close() error {
	result := t.buffer.String()

	// File Output
	if t.opts.OutPath != "" {
		if err := os.WriteFile(t.opts.OutPath, t.buffer.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to save tree file: %w", err)
		}
		fmt.Printf("\n📄 Tree saved to: %s", t.opts.OutPath)
	}

	// Clipboard Output
	if t.opts.CopyToClipboard {
		if err := clipboard.WriteAll(result); err != nil {
			return fmt.Errorf("failed to copy to clipboard: %w", err)
		}
		fmt.Println("\n📋 Tree copied to clipboard!")
	}
	fmt.Println()
	return nil
}

