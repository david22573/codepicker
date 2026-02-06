package indexer

import (
	"os"
	"testing"
)

func TestCodeSlicer_SliceFile(t *testing.T) {
	// Create a temporary Go file to test
	testFile := "test_slice.go"
	content := []byte(`package test
	
// MyStruct is a test struct
type MyStruct struct {
	ID int
}

func (m *MyStruct) GetID() int {
	return m.ID
}
`)
	os.WriteFile(testFile, content, 0644)
	defer os.Remove(testFile)

	slicer := NewCodeSlicer()
	slices, err := slicer.SliceFile(testFile)

	if err != nil {
		t.Fatalf("Slicing failed: %v", err)
	}

	if len(slices) != 2 {
		t.Errorf("Expected 2 slices, got %d", len(slices))
	}

	// Check if we found the struct and the method
	foundStruct := false
	foundMethod := false
	for _, s := range slices {
		if s.Symbols[0] == "MyStruct" {
			foundStruct = true
		}
		if s.Symbols[0] == "GetID" {
			foundMethod = true
		}
	}

	if !foundStruct || !foundMethod {
		t.Error("Failed to identify struct or method symbols")
	}
}
