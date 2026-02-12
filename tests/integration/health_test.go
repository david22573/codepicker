package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/david22573/codepicker/infra/metrics"
)

func TestMetricsServer_Endpoints(t *testing.T) {
	// 1. Setup: Start server on a random high port
	port := 58080
	srv := metrics.NewServer(port)
	srv.Start()

	// Ensure cleanup
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	}()

	// Wait briefly for server start
	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://localhost:%d", port)
	client := &http.Client{Timeout: 1 * time.Second}

	// 2. Define Test Cases
	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{"Liveness Probe", "/health", http.StatusOK},
		{"Readiness Probe", "/ready", http.StatusOK},
		{"Prometheus Metrics", "/metrics", http.StatusOK},
		{"Invalid Path", "/foobar", http.StatusNotFound},
	}

	// 3. Execute Requests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(baseURL + tt.path)
			if err != nil {
				t.Fatalf("Request to %s failed: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.name, tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestMetricsServer_ReadinessToggle(t *testing.T) {
	port := 58081
	srv := metrics.NewServer(port)
	srv.Start()
	defer srv.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/ready", port)

	// Case 1: Initially Ready
	resp, _ := http.Get(url)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected initial state to be 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Case 2: Toggle to Not Ready
	srv.SetReady(false)
	resp, _ = http.Get(url)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected state to be 503 Unavailable, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Case 3: Toggle back
	srv.SetReady(true)
	resp, _ = http.Get(url)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected state to be 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
