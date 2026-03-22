package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/david22573/codepicker/infra/llm"
	"github.com/spf13/cobra"
)

// Phase 0: Pack Command Configuration
var (
	packOutput      string
	packMode        string // "auto", "full", "smart"
	packMaxBytes    int64  // Hard cap for Full Mode (default 20MB)
	packMaxTokens   int    // Budget for Smart Mode
	packFormat      string // "xml" or "markdown"
	packTree        bool   // include file tree
	packSplit       bool   // Split output into multiple files
	packSplitTokens int    // Max tokens per split file
)

const (
	// Auto-switch threshold: 3MB
	AutoThresholdBytes = 3 * 1024 * 1024

	// Hard cap for Full Mode: 20MB
	HardCapBytes = 20 * 1024 * 1024

	// Ignore file name
	IgnoreFileName = ".codepickerignore"
)

var packCmd = &cobra.Command{
	Use:   "pack [targets...]",
	Short: "Optimize codebase context for LLM input",
	Long: `Consolidates your project into a high-density format for AI context.
Implements a Dual Pack Strategy:
  - Full Mode: Includes ALL files (non-truncated) up to a hard size cap.
  - Smart Mode: Budget-aware packing for larger repositories.
  - Auto Mode: Selects based on repository size (< 3MB = Full).
Respects .codepickerignore patterns.
You can specify particular files or directories to pack. If none are provided, it packs the entire current directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		targets := args
		if len(targets) == 0 {
			targets = []string{cwd}
			fmt.Printf("📦 Scanning entire project at: %s\n", cwd)
		} else {
			fmt.Printf("📦 Scanning specific targets: %v\n", targets)
		}

		// 1. Load Ignore Patterns
		ignorePatterns, err := loadIgnorePatterns(cwd)
		if err != nil {
			fmt.Printf("⚠️  Could not read %s: %v\n", IgnoreFileName, err)
		} else if len(ignorePatterns) > 0 {
			fmt.Printf("🚫 Loaded %d ignore rules from %s\n", len(ignorePatterns), IgnoreFileName)
		}

		// 2. Scan Targets & Calculate Size
		files, totalBytes, err := scanTargets(cwd, targets, ignorePatterns)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			fmt.Println("⚠️  No files found to pack.")
			return nil
		}

		fmt.Printf("📊 Detected: %d files, %s total size\n", len(files), formatBytes(totalBytes))

		// 3. Determine Strategy
		selectedMode := packMode
		if selectedMode == "auto" {
			if totalBytes < AutoThresholdBytes {
				selectedMode = "full"
				fmt.Println("🤖 Auto-Strategy: Total size is small (< 3MB) -> Using FULL Pack Mode")
			} else {
				selectedMode = "smart"
				fmt.Println("🤖 Auto-Strategy: Total size is large (> 3MB) -> Using SMART Pack Mode")
			}
		}

		// 4. Execute Strategy
		var tokenEst int
		var finalBytes int64

		if selectedMode == "full" {
			if totalBytes > int64(packMaxBytes) {
				return fmt.Errorf("❌ Full Pack Refused: Total size (%s) exceeds limit (%s). Use --mode=smart or increase --max-bytes",
					formatBytes(totalBytes), formatBytes(int64(packMaxBytes)))
			}
			tokenEst, finalBytes, err = runFullPack(cwd, files, packOutput)
		} else {
			tokenEst, finalBytes, err = runSmartPack(cwd, files, packOutput, packMaxTokens)
		}

		if err != nil {
			return err
		}

		// 5. Pack Manifest
		manifest := PackManifest{
			TotalBytes:      finalBytes,
			FileCount:       len(files),
			EstimatedTokens: tokenEst,
			Mode:            selectedMode,
			GeneratedAt:     time.Now(),
		}
		if err := appendManifest(packOutput, manifest); err != nil {
			fmt.Printf("⚠️ Failed to append manifest: %v\n", err)
		}

		fmt.Printf("\n✅ Pack Complete!\n")
		fmt.Printf("   Mode: %s\n", selectedMode)
		fmt.Printf("   Est. Tokens: ~%d\n", tokenEst)

		// 6. Post-Process Splitting
		if packSplit {
			fmt.Printf("\n🔪 Splitting output into ~%d token chunks...\n", packSplitTokens)
			if err := splitPackedFile(packOutput, packSplitTokens); err != nil {
				return fmt.Errorf("failed to split packed file: %w", err)
			}
		} else {
			fmt.Printf("   Output: %s\n", packOutput)
		}

		return nil
	},
}

func init() {
	packCmd.Flags().StringVarP(&packOutput, "output", "o", "codepicker_context.txt", "Output filename")
	packCmd.Flags().StringVar(&packMode, "mode", "auto", "Strategy: 'auto', 'full', 'smart'")
	packCmd.Flags().Int64Var(&packMaxBytes, "max-bytes", HardCapBytes, "Maximum size for Full Mode (default 20MB)")
	packCmd.Flags().IntVar(&packMaxTokens, "max-tokens", 32000, "Token budget for Smart Mode")
	packCmd.Flags().StringVar(&packFormat, "format", "xml", "Output format: 'xml' (recommended) or 'markdown'")
	packCmd.Flags().BoolVar(&packTree, "tree", true, "Include a directory tree at the top")
	packCmd.Flags().BoolVar(&packSplit, "split", false, "Automatically split the output into multiple parts if it exceeds a token limit")
	packCmd.Flags().IntVar(&packSplitTokens, "split-tokens", 30000, "Maximum tokens per file if --split is enabled")

	// Assuming rootCmd is defined elsewhere in your cmd package
	rootCmd.AddCommand(packCmd)
}

// --- Data Structures ---

type FileEntry struct {
	Path    string
	RelPath string
	Info    fs.FileInfo
	Tokens  int // estimated
}

type PackManifest struct {
	TotalBytes      int64     `json:"total_bytes"`
	FileCount       int       `json:"file_count"`
	EstimatedTokens int       `json:"estimated_tokens"`
	Mode            string    `json:"mode"`
	GeneratedAt     time.Time `json:"generated_at"`
}

// --- Core Logic ---

func loadIgnorePatterns(root string) ([]string, error) {
	path := filepath.Join(root, IgnoreFileName)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

func scanTargets(cwd string, targets []string, ignorePatterns []string) ([]FileEntry, int64, error) {
	var files []FileEntry
	var totalBytes int64
	seen := make(map[string]bool)

	for _, target := range targets {
		absTarget, err := filepath.Abs(target)
		if err != nil {
			fmt.Printf("⚠️  Could not resolve target %s: %v\n", target, err)
			continue
		}

		err = filepath.Walk(absTarget, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Prevent duplicate processing if paths overlap
			if seen[path] {
				return nil
			}
			seen[path] = true

			rel, err := filepath.Rel(cwd, path)
			if err != nil {
				rel = path // fallback if it can't be made relative
			}

			// Skip output file and .git immediately
			if info.Name() == packOutput || info.Name() == ".git" || info.Name() == "codepicker_out" {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// 1. Check Custom Ignores (.codepickerignore)
			if isIgnored(rel, ignorePatterns) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// 2. Default Ignores (Hardcoded safety)
			if info.IsDir() {
				name := info.Name()
				if (strings.HasPrefix(name, ".") && name != ".") ||
					name == "vendor" ||
					name == "node_modules" ||
					name == "dist" ||
					name == "bin" {
					return filepath.SkipDir
				}
				return nil
			}

			// 3. Filter Files (Extensions & Whitelist)
			if !shouldPack(path, info) {
				return nil
			}

			files = append(files, FileEntry{Path: path, RelPath: rel, Info: info})
			totalBytes += info.Size()

			return nil
		})

		if err != nil {
			return nil, 0, err
		}
	}

	return files, totalBytes, nil
}

func isIgnored(relPath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	// Normalize path separators for consistency
	relPath = filepath.ToSlash(relPath)

	for _, p := range patterns {
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}

		if strings.HasSuffix(p, "/") {
			dirName := strings.TrimSuffix(p, "/")
			if strings.HasPrefix(relPath, dirName) {
				return true
			}
			continue
		}

		if relPath == p {
			return true
		}

		matched, _ := filepath.Match(p, filepath.Base(relPath))
		if matched {
			return true
		}
	}
	return false
}

func runFullPack(root string, files []FileEntry, outFile string) (int, int64, error) {
	f, err := os.Create(outFile)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	totalChars := 0
	var writtenBytes int64 = 0

	// --- Header ---
	header := fmt.Sprintf("Project Context Dump (%s)\nFormat: %s\nMode: FULL (No Truncation)\nTotal Files: %d\n\n",
		time.Now().Format(time.RFC822), packFormat, len(files))
	w.WriteString(header)
	totalChars += len(header)

	// --- 1. The Tree ---
	if packTree {
		tree := generateTree(files)
		if packFormat == "xml" {
			w.WriteString("# Project Structure\n<file_tree>\n" + tree + "</file_tree>\n\n")
		} else {
			w.WriteString("# Project Structure\n```\n" + tree + "```\n\n")
		}
		totalChars += len(tree)
	}

	// --- 2. The Content ---
	w.WriteString("# File Contents\n")

	for _, file := range files {
		content, err := os.ReadFile(file.Path)
		if err != nil {
			fmt.Printf("⚠️  Skipping %s: read error\n", file.RelPath)
			continue
		}

		if !utf8.Valid(content) {
			fmt.Printf("⚠️  Skipping %s: binary detected\n", file.RelPath)
			continue
		}

		strContent := string(content)
		charsWritten := writeEntry(w, file.RelPath, strContent)
		totalChars += charsWritten
		writtenBytes += int64(len(content))

		fmt.Printf("  ➕ Packed: %s\n", file.RelPath)
	}

	w.Flush()

	estimator := llm.NewDefaultEstimator()
	// Using a simulated string length to estimate tokens, since we're appending to a writer
	return estimator.EstimateText(strings.Repeat("a", totalChars)), writtenBytes, nil
}

func runSmartPack(root string, files []FileEntry, outFile string, budget int) (int, int64, error) {
	f, err := os.Create(outFile)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	totalChars := 0
	var writtenBytes int64 = 0

	// Sort files by relevance
	sort.Slice(files, func(i, j int) bool {
		return scoreFile(files[i]) > scoreFile(files[j])
	})

	header := fmt.Sprintf("Project Context Dump (%s)\nFormat: %s\nMode: SMART (Budget: %d tokens)\n\n",
		time.Now().Format(time.RFC822), packFormat, budget)
	w.WriteString(header)
	totalChars += len(header)

	if packTree {
		tree := generateTree(files)
		if packFormat == "xml" {
			w.WriteString("<file_tree>\n" + tree + "</file_tree>\n\n")
		}
		totalChars += len(tree)
	}

	estimator := llm.NewDefaultEstimator()
	usedTokens := estimator.EstimateText(strings.Repeat("a", totalChars))

	for _, file := range files {
		content, err := os.ReadFile(file.Path)
		if err != nil || !utf8.Valid(content) {
			continue
		}

		strContent := string(content)
		fileTokens := estimator.EstimateText(strContent)

		if usedTokens+fileTokens > budget {
			fmt.Printf("  ➖ Skipped (Budget): %s (%d tokens)\n", file.RelPath, fileTokens)
			continue
		}

		writeEntry(w, file.RelPath, strContent)
		usedTokens += fileTokens
		writtenBytes += int64(len(content))
		fmt.Printf("  ➕ Packed: %s\n", file.RelPath)
	}

	w.Flush()
	return usedTokens, writtenBytes, nil
}

func splitPackedFile(inputPath string, maxTokens int) error {
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}

	estimator := llm.NewDefaultEstimator()
	reader := bufio.NewReader(file)

	var currentChunk strings.Builder
	var currentTokens int
	partNumber := 1

	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	dir := filepath.Dir(inputPath)

	writeChunk := func(content string, tokens int) error {
		if content == "" {
			return nil
		}
		outName := fmt.Sprintf("%s_part%d%s", nameWithoutExt, partNumber, ext)
		outPath := filepath.Join(dir, outName)

		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			return err
		}

		fmt.Printf("  📄 Created %s (~%d tokens)\n", outName, tokens)
		partNumber++
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			file.Close()
			return err
		}

		isEOF := (err == io.EOF)

		if line != "" {
			lineTokens := estimator.EstimateText(line)

			if currentTokens+lineTokens > maxTokens && currentChunk.Len() > 0 {
				if err := writeChunk(currentChunk.String(), currentTokens); err != nil {
					file.Close()
					return err
				}
				currentChunk.Reset()
				currentTokens = 0
			}

			currentChunk.WriteString(line)
			currentTokens += lineTokens
		}

		if isEOF {
			break
		}
	}

	if currentChunk.Len() > 0 {
		if err := writeChunk(currentChunk.String(), currentTokens); err != nil {
			file.Close()
			return err
		}
	}

	// Cleanup the monolithic file
	file.Close()
	if err := os.Remove(inputPath); err != nil {
		fmt.Printf("  ⚠️ Could not remove original monolithic file %s: %v\n", inputPath, err)
	} else {
		fmt.Printf("  🧹 Removed original un-split file to save space.\n")
	}

	return nil
}

// --- Helpers ---

func writeEntry(w *bufio.Writer, relPath, content string) int {
	var out string
	if packFormat == "xml" {
		out = fmt.Sprintf("<file path=\"%s\">\n%s\n</file>\n\n", relPath, content)
	} else {
		ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
		if ext == "" {
			ext = "txt"
		}
		out = fmt.Sprintf("## File: %s\n```%s\n%s\n```\n\n", relPath, ext, content)
	}
	w.WriteString(out)
	return len(out)
}

func appendManifest(path string, m PackManifest) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, _ := json.MarshalIndent(m, "", "  ")
	_, err = f.WriteString("\n\n```json\n" + string(data) + "\n```\n")
	return err
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func generateTree(files []FileEntry) string {
	var sb strings.Builder
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s\n", f.RelPath))
	}
	return sb.String()
}

func shouldPack(path string, info fs.FileInfo) bool {
	// Skip hidden files
	if strings.HasPrefix(info.Name(), ".") {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	allowed := map[string]bool{
		".go": true, ".mod": true, ".sum": true,
		".md": true, ".json": true, ".yaml": true, ".yml": true,
		".sql": true, ".sh": true, ".txt": true, ".toml": true,
		".html": true, ".css": true, ".js": true, ".ts": true,
		".tsx": true, ".jsx": true, ".py": true, ".c": true, ".h": true,
		".dockerfile": true, ".svelte": true, ".vue": true, ".rb": true,
		".php": true, ".java": true, ".kt": true, ".swift": true, ".rs": true,
		".lua": true, ".dart": true, ".scala": true, ".pl": true, ".pm": true,
	}

	specialFiles := map[string]bool{
		"makefile": true, "dockerfile": true, "license": true,
		"readme": true, "changelog": true, "notice": true,
	}

	if allowed[ext] {
		return true
	}
	if specialFiles[strings.ToLower(info.Name())] {
		return true
	}
	return false
}

func scoreFile(f FileEntry) int {
	score := 0
	base := strings.ToLower(filepath.Base(f.RelPath))

	if base == "main.go" {
		score += 100
	}
	if strings.HasSuffix(base, ".md") {
		score += 50
	}
	if strings.Contains(f.RelPath, "domain") {
		score += 40
	}
	if strings.Contains(f.RelPath, "cmd") {
		score += 30
	}
	if strings.HasSuffix(base, "_test.go") {
		score -= 50
	}
	return score
}
