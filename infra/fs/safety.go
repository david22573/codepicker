package fs

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	MaxFileSize    = 10 * 1024 * 1024 // 10MB limit
	MaxReadTimeout = 5 * time.Second  //
)

// SafeReadFile checks file size and type before reading to prevent OOM or hangs.
func SafeReadFile(ctx context.Context, path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes), limit is %d", info.Size(), MaxFileSize)
	}

	// Create a timeout specifically for the I/O operation
	readCtx, cancel := context.WithTimeout(ctx, MaxReadTimeout)
	defer cancel()

	// Read file in a goroutine to respect timeout context
	resultChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	go func() {
		content, err := os.ReadFile(path)
		if err != nil {
			errChan <- err
			return
		}

		// Binary detection via Sniffing
		if len(content) > 512 {
			contentType := http.DetectContentType(content[:512])
			if isForbiddenBinary(contentType) {
				errChan <- fmt.Errorf("binary file detected (type: %s)", contentType)
				return
			}
		}

		resultChan <- content
	}()

	select {
	case <-readCtx.Done():
		return nil, fmt.Errorf("read operation timed out for %s", path)
	case err := <-errChan:
		return nil, err
	case content := <-resultChan:
		return content, nil
	}
}

func isForbiddenBinary(contentType string) bool {
	forbidden := []string{"application/octet-stream", "application/x-executable", "application/zip"}
	for _, f := range forbidden {
		if contentType == f {
			return true
		}
	}
	return false
}
