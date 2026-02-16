package fs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	MaxFileSize    = 10 * 1024 * 1024 // 10MB limit
	MaxReadTimeout = 5 * time.Second  // Hard timeout for I/O operations
)

// SafeReadFile reads a file with strict size limits, binary detection, and timeout enforcement.
// It prevents goroutine leaks by actively closing the file handle if a timeout occurs.
func SafeReadFile(ctx context.Context, path string) ([]byte, error) {
	// 1. Stat check for initial size limit
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes), limit is %d", info.Size(), MaxFileSize)
	}

	// 2. Open the file
	// We do NOT defer f.Close() here immediately because we might need to close it
	// asynchronously to interrupt the read on timeout.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// Channel to capture the result or error from the read operation
	type readResult struct {
		data []byte
		err  error
	}
	resultChan := make(chan readResult, 1)

	// 3. Run Read in a separate Goroutine
	go func() {
		// Ensure file is closed when this goroutine finishes (normal path)
		defer f.Close()

		// A. Read first 512 bytes for MIME sniffing
		header := make([]byte, 512)
		n, err := f.Read(header)
		if err != nil && err != io.EOF {
			resultChan <- readResult{nil, err}
			return
		}

		// B. Binary Detection
		if n > 0 {
			contentType := http.DetectContentType(header[:n])
			if isForbiddenBinary(contentType) {
				resultChan <- readResult{nil, fmt.Errorf("binary file detected (type: %s)", contentType)}
				return
			}
		}

		// If file was smaller than 512 bytes, we are done
		if n < 512 {
			resultChan <- readResult{header[:n], nil}
			return
		}

		// C. Read the rest with a hard limit constraint
		// We use a buffer initialized with the header we already read
		var buf bytes.Buffer
		buf.Write(header[:n])

		// Calculate remaining allowed bytes
		remainingLimit := MaxFileSize - int64(n)

		// Use LimitReader to prevent memory exhaustion if file grows during read
		if _, err := io.Copy(&buf, io.LimitReader(f, remainingLimit)); err != nil {
			resultChan <- readResult{nil, err}
			return
		}

		resultChan <- readResult{buf.Bytes(), nil}
	}()

	// 4. Wait for Result or Timeout
	select {
	case res := <-resultChan:
		return res.data, res.err

	case <-ctx.Done():
		// TIMEOUT/CANCEL: Explicitly Close() the file.
		// This forces the blocked Read() in the goroutine to error out,
		// allowing the goroutine to exit and preventing a leak.
		_ = f.Close()
		return nil, fmt.Errorf("read operation cancelled for %s: %w", path, ctx.Err())

	case <-time.After(MaxReadTimeout):
		// Internal safety timeout
		_ = f.Close()
		return nil, fmt.Errorf("read operation timed out (exceeded %s)", MaxReadTimeout)
	}
}

func isForbiddenBinary(contentType string) bool {
	forbidden := []string{
		"application/octet-stream",
		"application/x-executable",
		"application/zip",
		"application/x-gzip",
		"application/x-tar",
	}
	for _, f := range forbidden {
		if contentType == f {
			return true
		}
	}
	return false
}
