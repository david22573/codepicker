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
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/minifier"
	"github.com/david22573/codepicker/internal/tokenizer"
)

// --- Interfaces ---

type OutputStrategy interface {
	Init() error
	Write(absPath, relPath string) error
	Close() error
	ShouldSkip(path string) bool
	Name() string
}

// --- Concat Strategy (Combines files into one) ---

type ConcatStrategy struct {
	OutputPath    string
	absOutputPath string // Cached absolute path for safer comparison
	file          *os.File
	TokenCount    int
	Minify        bool
	ComputeTokens bool
}

func NewConcatStrategy(path string, minify bool, computeTokens bool) *ConcatStrategy {
	abs, _ := filepath.Abs(path)
	return &ConcatStrategy{
		OutputPath:    path,
		absOutputPath: abs,
		Minify:        minify,
		ComputeTokens: computeTokens,
	}
}

func (c *ConcatStrategy) Name() string { return "Concat" }

func (c *ConcatStrategy) Init() error {
	if err := os.MkdirAll(filepath.Dir(c.OutputPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(c.OutputPath)
	if err != nil {
		return err
	}
	c.file = f
	fmt.Fprintf(c.file, constants.ContextHeader)
	fmt.Fprintf(c.file, constants.GeneratedBy)
	return nil
}

func (c *ConcatStrategy) ShouldSkip(path string) bool {
	// FIX: Robust check. Convert the candidate path to absolute
	// so we don't accidentally scan our own output file.
	absCandidate, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return absCandidate == c.absOutputPath
}

func (c *ConcatStrategy) Write(absPath, relPath string) error {
	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() > constants.MaxFileSize {
		fmt.Printf("⚠️  Skipping large file (>%dMB): %s\n", constants.MaxFileSize/(1024*1024), relPath)
		return nil
	}

	// Read file into memory (Safe on Termux)
	content, err := io.ReadAll(io.LimitReader(f, constants.MaxFileSize))
	if err != nil {
		return err
	}

	if !utf8.Valid(content) {
		return nil
	}

	if c.Minify {
		ext := strings.ToLower(filepath.Ext(relPath))
		content = minifier.Minify(content, ext)
	}

	ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
	if ext == "" {
		ext = "text"
	}

	var fileBuffer bytes.Buffer
	fmt.Fprintf(&fileBuffer, "## File: %s\n", relPath)
	fmt.Fprintf(&fileBuffer, "```%s\n", ext)
	fileBuffer.Write(content)

	if len(content) > 0 && content[len(content)-1] != '\n' {
		fileBuffer.Write([]byte("\n"))
	}
	fmt.Fprintf(&fileBuffer, "```\n\n")

	finalBytes := fileBuffer.Bytes()

	if c.ComputeTokens {
		c.TokenCount += tokenizer.CountTokens(string(finalBytes))
	}

	_, err = c.file.Write(finalBytes)
	return err
}

func (c *ConcatStrategy) Close() error {
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}

// --- Copy Strategy (Copies files preserving structure) ---

type CopyStrategy struct {
	OutputDir string
}

func NewCopyStrategy(dir string) *CopyStrategy {
	return &CopyStrategy{OutputDir: dir}
}

func (c *CopyStrategy) Name() string { return "Copy" }

func (c *CopyStrategy) Init() error {
	return os.MkdirAll(c.OutputDir, 0755)
}

func (c *CopyStrategy) ShouldSkip(path string) bool {
	// Prevent copying the output folder into itself
	rel, err := filepath.Rel(c.OutputDir, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func (c *CopyStrategy) Write(absPath, relPath string) error {
	targetPath := filepath.Join(c.OutputDir, relPath)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir for %s: %w", relPath, err)
	}

	src, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("failed to open source %s: %w", absPath, err)
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target %s: %w", targetPath, err)
	}
	defer dst.Close()

	// FIX: CRITICAL FOR TERMUX/ANDROID
	// io.Copy uses 'sendfile' syscall which fails on Android with "invalid argument".
	// io.CopyBuffer forces a userspace copy which is 100% compatible.
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		return fmt.Errorf("failed to copy content to %s: %w", targetPath, err)
	}

	return nil
}

func (c *CopyStrategy) Close() error { return nil }

// --- Tree Strategy (Visual tree) ---

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

func (t *TreeStrategy) Name() string { return "Tree" }

func (t *TreeStrategy) Init() error {
	header := "PROJECT STRUCTURE:\n.\n"
	fmt.Print(header)
	t.buffer.WriteString(header)
	return nil
}

func (t *TreeStrategy) ShouldSkip(path string) bool {
	if t.opts.OutPath != "" {
		// Use suffix check as a simple heuristic
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
	fmt.Print(line)
	t.buffer.WriteString(line)
	return nil
}

func (t *TreeStrategy) Close() error {
	result := t.buffer.String()
	if t.opts.OutPath != "" {
		if err := os.WriteFile(t.opts.OutPath, t.buffer.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to save tree file: %w", err)
		}
		fmt.Printf("\n📄 Tree saved to: %s", t.opts.OutPath)
	}
	if t.opts.CopyToClipboard {
		if err := clipboard.WriteAll(result); err != nil {
			return fmt.Errorf("failed to copy to clipboard: %w", err)
		}
		fmt.Println("\n📋 Tree copied to clipboard!")
	}
	fmt.Println()
	return nil
}

