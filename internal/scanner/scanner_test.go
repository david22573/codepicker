package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
)

// MockWriter implements writer.OutputStrategy for testing
type MockWriter struct {
	Files []string
}

func (m *MockWriter) Init() error                 { return nil }
func (m *MockWriter) Close() error                { return nil }
func (m *MockWriter) ShouldSkip(path string) bool { return false }
func (m *MockWriter) Name() string                { return "Mock" }
func (m *MockWriter) Write(abs, rel string) error {
	m.Files = append(m.Files, rel)
	return nil
}

func TestScanner(t *testing.T) {
	// 1. Setup Temp Dir
	tmpDir := t.TempDir()

	// 2. Create Dummy Files
	createFile(t, tmpDir, "main.go", "package main")
	createFile(t, tmpDir, "utils.go", "package main")
	createFile(t, tmpDir, "ignored.log", "log data")

	subDir := filepath.Join(tmpDir, "internal")
	os.Mkdir(subDir, 0755)
	createFile(t, subDir, "logic.go", "package logic")

	vendorDir := filepath.Join(tmpDir, "vendor")
	os.Mkdir(vendorDir, 0755)
	createFile(t, vendorDir, "dep.go", "package dep")

	// 3. Configure Scanner
	cfg := config.NewConfig()
	// Default config already includes .go and excludes vendor

	mockW := &MockWriter{}
	nopLogger := &logger.NoOpLogger{}

	s := NewScanner(tmpDir, mockW, cfg, nopLogger)

	// 4. Run Scan
	err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// 5. Assertions
	expected := map[string]bool{
		"main.go":           true,
		"utils.go":          true,
		"internal/logic.go": true,
	}

	unexpected := map[string]bool{
		"ignored.log":   true, // Not in allowed extensions
		"vendor/dep.go": true, // In ignored dirs
	}

	// Windows path adjustment for tests running on Windows
	found := make(map[string]bool)
	for _, f := range mockW.Files {
		found[filepath.ToSlash(f)] = true
	}

	for f := range expected {
		if !found[f] {
			t.Errorf("Expected to find file %s, but did not. Found: %v", f, mockW.Files)
		}
	}

	for f := range unexpected {
		if found[f] {
			t.Errorf("Found unexpected file %s (should have been ignored)", f)
		}
	}
}

func createFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", path, err)
	}
}
