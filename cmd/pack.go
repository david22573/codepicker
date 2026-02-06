package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

var (
	packOutput  string
	packMaxSize int64
	packFormat  string // "markdown" or "xml"
	packTree    bool   // include file tree
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Optimize codebase context for LLM input",
	Long:  `Consolidates your project into a high-density format for AI context. Includes a file tree and optional XML framing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		fmt.Printf("📦 Packing project from: %s\n", cwd)

		// 1. Capture the file list first (for tree generation and walking)
		files, err := scanProject(cwd, packMaxSize)
		if err != nil {
			return err
		}

		// 2. Run the packer
		tokenEst, err := runPack(cwd, files, packOutput)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Done! Output: %s\n", packOutput)
		fmt.Printf("📊 Est. Tokens: ~%d (based on 4 chars/token)\n", tokenEst)
		return nil
	},
}

func init() {
	packCmd.Flags().StringVarP(&packOutput, "output", "o", "codepicker_context.txt", "Output filename")
	packCmd.Flags().Int64Var(&packMaxSize, "max-size", 1024*1024, "Max file size in bytes (default 1MB)")
	packCmd.Flags().StringVar(&packFormat, "format", "xml", "Output format: 'xml' (best for agents) or 'markdown' (readable)")
	packCmd.Flags().BoolVar(&packTree, "tree", true, "Include a directory tree at the top")

	rootCmd.AddCommand(packCmd)
}

// FileEntry holds metadata for sorting and processing
type FileEntry struct {
	Path    string
	RelPath string
	Info    fs.DirEntry
}

func scanProject(root string, limit int64) ([]FileEntry, error) {
	var files []FileEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// 1. Skip Hidden Dirs & Heavy Dependencies
		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && name != ".") ||
				name == "vendor" ||
				name == "node_modules" ||
				name == "dist" ||
				name == "bin" ||
				name == "codepicker_out" { // Skip output dirs
				return filepath.SkipDir
			}
			return nil
		}

		// 2. Filter Files
		if !shouldPack(path, d, limit) {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		files = append(files, FileEntry{Path: path, RelPath: rel, Info: d})
		return nil
	})

	return files, err
}

func runPack(root string, files []FileEntry, outFile string) (int, error) {
	f, err := os.Create(outFile)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	totalChars := 0

	// --- Header ---
	header := fmt.Sprintf("Project Context Dump (%s)\nFormat: %s\nTotal Files: %d\n\n",
		time.Now().Format(time.RFC822), packFormat, len(files))
	w.WriteString(header)

	// --- 1. The Tree (The Map) ---
	if packTree {
		w.WriteString("# Project Structure\n")
		w.WriteString("<file_tree>\n")
		tree := generateTree(files)
		w.WriteString(tree)
		w.WriteString("</file_tree>\n\n")
		totalChars += len(tree)
	}

	// --- 2. The Content ---
	w.WriteString("# File Contents\n")

	for _, file := range files {
		// Skip the output file itself if it was caught
		if file.RelPath == outFile {
			continue
		}

		content, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}

		// Binary check
		if !utf8.Valid(content) {
			continue
		}

		strContent := string(content)
		totalChars += len(strContent)

		if packFormat == "xml" {
			// XML Style: <file path="cmd/main.go"> ... </file>
			// This is extremely robust for LLM parsing.
			fmt.Fprintf(w, "<file path=\"%s\">\n%s\n</file>\n\n", file.RelPath, strContent)
		} else {
			// Markdown Style: ## File: cmd/main.go
			ext := strings.TrimPrefix(filepath.Ext(file.Path), ".")
			if ext == "" {
				ext = "txt"
			}

			fmt.Fprintf(w, "## File: %s\n```%s\n%s\n```\n\n", file.RelPath, ext, strContent)
		}

		fmt.Printf("  ➕ Packed: %s\n", file.RelPath)
	}

	w.Flush()

	// Rough token estimate (Char / 4)
	return totalChars / 4, nil
}

// generateTree creates a visual string representation of the file list
func generateTree(files []FileEntry) string {
	var sb strings.Builder
	// Sort to keep directories grouped
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s\n", f.RelPath))
	}
	return sb.String()
}

func shouldPack(path string, d fs.DirEntry, limit int64) bool {
	// Skip hidden files
	if strings.HasPrefix(d.Name(), ".") {
		return false
	}

	// Check size
	info, err := d.Info()
	if err != nil || info.Size() > limit {
		return false
	}

	// Whitelist Extensions
	ext := strings.ToLower(filepath.Ext(path))
	allowed := map[string]bool{
		".go": true, ".mod": true, ".sum": true,
		".md": false, ".json": true, ".yaml": true, ".yml": true,
		".sql": true, ".sh": true, ".txt": true, ".toml": true,
		".html": true, ".css": true, ".js": true, ".ts": true,
		".tsx": true, ".jsx": true, ".py": true, ".c": true, ".h": true,
		".dockerfile": false,
	}

	specialFiles := map[string]bool{
		"makefile": true, "dockerfile": true, "license": true,
		"readme": true, "changelog": true, "notice": true,
	}

	if allowed[ext] {
		return true
	}
	if specialFiles[strings.ToLower(d.Name())] {
		return true
	}

	return false
}
