package minifier

import (
	"strings"
	"testing"
)

func TestMinify(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		ext      string
		contains []string
		excludes []string
	}{
		{
			name: "Go Minification (AST-based)",
			content: `package main
// This is a comment
func main() {
	println("hello")
}`,
			ext:      ".go",
			contains: []string{"package main", "func main", "println"},
			excludes: []string{"// This is a comment"}, // Go minifier SHOULD still strip comments
		},
		{
			name: "JS Minification (Safe Mode)",
			content: `function test() {
    // a comment
    return true;
}`,
			ext:      ".js",
			contains: []string{"function test", "return true", "// a comment"}, // Safe mode KEEPS comments
			excludes: []string{"\n\n"},                                         // Should remove empty vertical space
		},
		{
			name: "Python Minification (Safe Mode)",
			content: `def foo():
    # python comment
    print("bar")

`,
			ext:      ".py",
			contains: []string{"def foo", "print", "# python comment"}, // Safe mode KEEPS comments
			excludes: []string{"\n\n"},                                 // Should remove empty vertical space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Minify([]byte(tt.content), tt.ext)
			gotStr := string(got)

			for _, c := range tt.contains {
				if !strings.Contains(gotStr, c) {
					t.Errorf("Expected output to contain '%s', but it didn't.\nOutput:\n%s", c, gotStr)
				}
			}

			for _, e := range tt.excludes {
				if strings.Contains(gotStr, e) {
					t.Errorf("Expected output to exclude '%s', but it remained.\nOutput:\n%s", e, gotStr)
				}
			}
		})
	}
}
