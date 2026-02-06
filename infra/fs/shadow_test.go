package fs

import (
	"os"
	"testing"
)

func TestShadowManager_Security(t *testing.T) {
	sm := NewShadowManager("/tmp/project")

	attacks := []string{"/etc/passwd", "../../secret", "cmd/../../evil"}
	for _, path := range attacks {
		_, err := sm.Write(path, []byte("test"))
		if err == nil {
			t.Errorf("Security Breach: failed to block path %s", path)
		}
	}
}

func TestShadowManager_Cycle(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "shadow-test-*")
	defer os.RemoveAll(tmp)

	sm := NewShadowManager(tmp)
	rel := "test.go"
	content := []byte("data")

	// Fix: handle BOTH return values
	path, err := sm.Write(rel, content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if path == "" {
		t.Fatal("Expected path, got empty string")
	}

	if err := sm.Commit(rel); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
