package writer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync" // Added for thread safety
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/minifier"
	"github.com/david22573/codepicker/internal/tokenizer"
)

type OutputStrategy interface {
	Init() error
	Write(absPath, relPath string) error
	Close() error
	ShouldSkip(path string) bool
	Name() string
}

// ConcatStrategy combines all files into one document.
type ConcatStrategy struct {
	OutputPath    string
	absOutputPath string
	file          *os.File
	TokenCount    int
	Minify        bool
	ComputeTokens bool
	mu            sync.Mutex // Mutex ensures atomic writes from concurrent workers
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

	f, err := os.OpenFile(c.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	c.file = f
	fmt.Fprintf(c.file, constants.ContextHeader)
	fmt.Fprintf(c.file, constants.GeneratedBy)
	return nil
}

func (c *ConcatStrategy) ShouldSkip(path string) bool {
	absCandidate, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return absCandidate == c.absOutputPath
}

// Write processes a single file. It is now thread-safe.
func (c *ConcatStrategy) Write(absPath, relPath string) error {
	// 1. Heavy lifting (IO + CPU) happens BEFORE the lock
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

	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return err
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	contentType := http.DetectContentType(header[:n])
	isBinary := strings.HasPrefix(contentType, "application/octet-stream")

	if n > 0 && isBinary {
		fmt.Printf("⚠️  Skipping binary file detected: %s (%s)\n", relPath, contentType)
		return nil
	}

	// Prepare content in memory buffer first
	var contentBuffer bytes.Buffer

	ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
	if ext == "" {
		ext = "text"
	}

	fmt.Fprintf(&contentBuffer, "## File: %s\n", relPath)
	fmt.Fprintf(&contentBuffer, "```%s\n", ext)

	// Read and Minify (CPU intensive work)
	rawContent, err := io.ReadAll(io.LimitReader(f, constants.MaxFileSize))
	if err != nil {
		return err
	}

	if !utf8.Valid(rawContent) {
		fmt.Printf("⚠️  Skipping invalid UTF-8 file: %s\n", relPath)
		return nil
	}

	var bytesToWrite []byte
	var tokens int

	if c.Minify {
		extStr := strings.ToLower(filepath.Ext(relPath))
		bytesToWrite = minifier.Minify(rawContent, extStr)
	} else {
		bytesToWrite = rawContent
	}

	if c.ComputeTokens {
		tokens = tokenizer.CountTokens(string(bytesToWrite))
	}

	contentBuffer.Write(bytesToWrite)
	contentBuffer.Write([]byte("\n```\n\n"))

	// 2. Critical Section: Write to shared file handle
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.file.Write(contentBuffer.Bytes()); err != nil {
		return err
	}

	if c.ComputeTokens {
		c.TokenCount += tokens
	}

	return nil
}

func (c *ConcatStrategy) Close() error {
	if c.file != nil {
		c.file.Sync()
		return c.file.Close()
	}
	return nil
}

// CopyStrategy writes to separate files, so it is inherently thread-safe
// as long as filenames are unique (which the scanner guarantees).
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

	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		return fmt.Errorf("failed to copy content to %s: %w", targetPath, err)
	}

	return nil
}

func (c *CopyStrategy) Close() error { return nil }

type TreeOptions struct {
	CopyToClipboard bool
	OutPath         string
}

type TreeStrategy struct {
	opts   TreeOptions
	buffer *bytes.Buffer
	mu     sync.Mutex // Protects the shared buffer
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

	t.mu.Lock()
	defer t.mu.Unlock()

	// Print to stdout immediately (might interleave slightly but acceptable for progress)
	fmt.Print(line)
	// Buffer writes must be atomic
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
