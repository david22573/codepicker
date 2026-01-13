package writer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// O_APPEND | O_CREATE | O_WRONLY ensures we don't accidentally wipe if logic fails elsewhere
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

func (c *ConcatStrategy) Write(absPath, relPath string) error {
	f, err := os.Open(absPath)
	if err != nil {
		return err // Logging handled by scanner
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

	// 1. Read header for Binary Detection
	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return err
	}
	// Reset file pointer after reading header
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	contentType := http.DetectContentType(header[:n])
	isBinary := strings.HasPrefix(contentType, "application/octet-stream")

	if n > 0 && isBinary {
		fmt.Printf("⚠️  Skipping binary file detected: %s (%s)\n", relPath, contentType)
		return nil
	}

	// 2. Prepare Header
	ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
	if ext == "" {
		ext = "text"
	}

	// Write file header to output (buffered to minimize syscalls)
	// We use a temporary buffer for the small headers to avoid partial writes
	var metaBuf bytes.Buffer
	fmt.Fprintf(&metaBuf, "## File: %s\n", relPath)
	fmt.Fprintf(&metaBuf, "```%s\n", ext)
	if _, err := c.file.Write(metaBuf.Bytes()); err != nil {
		return err
	}

	// 3. Process Content
	var bytesWritten int64

	// If Minification is ON, we must read the file into memory (AST parsing requires it).
	// We use io.LimitReader to strictly enforce the size limit.
	if c.Minify {
		content, err := io.ReadAll(io.LimitReader(f, constants.MaxFileSize))
		if err != nil {
			return err
		}

		if !utf8.Valid(content) {
			fmt.Printf("⚠️  Skipping invalid UTF-8 file: %s\n", relPath)
			return nil
		}

		extStr := strings.ToLower(filepath.Ext(relPath))
		minified := minifier.Minify(content, extStr)

		if _, err := c.file.Write(minified); err != nil {
			return err
		}
		bytesWritten = int64(len(minified))

		if c.ComputeTokens {
			c.TokenCount += tokenizer.CountTokens(string(minified))
		}

	} else {
		// STREAMING MODE (Memory Efficient)
		// We copy directly from source file to output file.
		// NOTE: Token counting in streaming mode is expensive (requires double read),
		// so we only estimate or skip it if strict accuracy isn't required.
		// For this implementation, we buffer the read to count tokens if needed.

		if c.ComputeTokens {
			// If we need tokens, we have to read it. Streaming + Counting is hard without a TeeReader and buffer.
			// Fallback to read-all for token counting accuracy, or implement a counting scanner.
			// For safety, we'll read-all but limited.
			content, err := io.ReadAll(io.LimitReader(f, constants.MaxFileSize))
			if err != nil {
				return err
			}

			if _, err := c.file.Write(content); err != nil {
				return err
			}
			c.TokenCount += tokenizer.CountTokens(string(content))
			bytesWritten = int64(len(content))
		} else {
			// Pure streaming - super fast, low memory
			written, err := io.Copy(c.file, io.LimitReader(f, constants.MaxFileSize))
			if err != nil {
				return err
			}
			bytesWritten = written
		}
	}

	// 4. Write Footer
	// Ensure newline before closing block
	// We can't easily check the last byte in streaming mode without complex logic,
	// so we force a newline for safety.
	if _, err := c.file.Write([]byte("\n```\n\n")); err != nil {
		return err
	}

	// Just for debug visibility
	if bytesWritten == 0 {
		// Log?
	}

	return nil
}

func (c *ConcatStrategy) Close() error {
	if c.file != nil {
		// Sync ensures data is flushed to disk before we close
		c.file.Sync()
		return c.file.Close()
	}
	return nil
}

// CopyStrategy and TreeStrategy remain largely the same, ensuring Copy uses io.CopyBuffer
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

	// Use CopyBuffer for efficiency
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
