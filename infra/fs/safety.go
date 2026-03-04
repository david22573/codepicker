package fs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	MaxFileSize    = 10 * 1024 * 1024
	MaxReadTimeout = 5 * time.Second
)

func SafeReadFile(ctx context.Context, path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Prevent goroutine leaks caused by pipes or devices
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}

	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes), limit is %d", info.Size(), MaxFileSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var closeOnce sync.Once
	closeFile := func() {
		closeOnce.Do(func() {
			_ = f.Close()
		})
	}

	type readResult struct {
		data []byte
		err  error
	}
	resultChan := make(chan readResult, 1)

	go func() {
		defer closeFile()

		header := make([]byte, 512)
		n, err := f.Read(header)
		if err != nil && err != io.EOF {
			resultChan <- readResult{nil, err}
			return
		}

		if n > 0 {
			contentType := http.DetectContentType(header[:n])
			if isForbiddenBinary(contentType) {
				resultChan <- readResult{nil, fmt.Errorf("binary file detected (type: %s)", contentType)}
				return
			}
		}

		if n < 512 {
			resultChan <- readResult{header[:n], nil}
			return
		}

		var buf bytes.Buffer
		buf.Write(header[:n])

		remainingLimit := MaxFileSize - int64(n)

		if _, err := io.Copy(&buf, io.LimitReader(f, remainingLimit)); err != nil {
			resultChan <- readResult{nil, err}
			return
		}

		resultChan <- readResult{buf.Bytes(), nil}
	}()

	select {
	case res := <-resultChan:
		return res.data, res.err

	case <-ctx.Done():
		closeFile()
		return nil, fmt.Errorf("read operation cancelled for %s: %w", path, ctx.Err())

	case <-time.After(MaxReadTimeout):
		closeFile()
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
