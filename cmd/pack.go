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

	"github.com/atotto/clipboard"
	"github.com/david22573/codepicker/infra/git"
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
	packSplitTokens int    // Max tokens per file
	packClipboard   bool   // Copy to clipboard
	packMeta        bool   // Include metadata and headers
	packTask        string
	packChanged     bool
	packProfile     string
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
Respects .codepickerignore patterns.
Includes structured headers, directory trees, and token summary tables.`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var origStdout *os.File
		if GetJSON() {
			origStdout = os.Stdout
			os.Stdout = os.Stderr
			defer func() {
				os.Stdout = origStdout
				if err != nil {
					packJSON := map[string]interface{}{
						"status": "fail",
						"error":  err.Error(),
					}
					jsonData, _ := json.Marshal(packJSON)
					fmt.Fprintln(origStdout, string(jsonData))
				}
			}()
		}

		var cwd string
		cwd, err = os.Getwd()
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

		// Check profile if specified
		var profile Profile
		if packProfile != "" {
			p, exists := profiles[packProfile]
			if !exists {
				var available []string
				for k := range profiles {
					available = append(available, k)
				}
				sort.Strings(available)
				return fmt.Errorf("unknown profile '%s'. Available: %v", packProfile, available)
			}
			profile = p
			fmt.Printf("📋 Using profile: %s\n", packProfile)
		}

		// Check changed files from Git if specified
		var changedFilesMap map[string]bool
		if packChanged {
			gitClient := git.NewClient(cwd, false)
			list, err := gitClient.GetChangedFiles(cmd.Context())
			if err != nil {
				fmt.Printf("⚠️  Failed to get changed files from Git: %v\n", err)
			} else {
				changedFilesMap = make(map[string]bool)
				for _, f := range list {
					changedFilesMap[filepath.ToSlash(f)] = true
				}
				fmt.Printf("🌿 Detected %d changed files in Git\n", len(changedFilesMap))
			}
		}

		// 1. Load Ignore Patterns
		ignorePatterns, err := loadIgnorePatterns(cwd)
		if err != nil {
			fmt.Printf("⚠️  Could not read %s: %v\n", IgnoreFileName, err)
		} else if len(ignorePatterns) > 0 {
			fmt.Printf("🚫 Loaded %d ignore rules from %s\n", len(ignorePatterns), IgnoreFileName)
		}

		// 2. Scan Targets & Calculate Size
		files, totalBytes, excludedCount, err := scanTargets(cwd, targets, ignorePatterns, changedFilesMap, profile)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			fmt.Println("⚠️  No files found to pack.")
			return nil
		}

		fmt.Printf("📊 Detected: %d files, %s total size (excluded %d)\n", len(files), formatBytes(totalBytes), excludedCount)

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

		// 4. Select and Sort Files Deterministically
		var selectedFiles []FileEntry
		estimator := llm.NewDefaultEstimator()

		if selectedMode == "full" {
			selectedFiles = files
		} else {
			// Smart Mode: Sort by score first to select the best files within budget
			sortedByScore := make([]FileEntry, len(files))
			copy(sortedByScore, files)
			sort.Slice(sortedByScore, func(i, j int) bool {
				return scoreFile(sortedByScore[i]) > scoreFile(sortedByScore[j])
			})

			var currentBudget int
			if packMaxTokens > 0 {
				currentBudget = packMaxTokens
			} else {
				currentBudget = 32000
			}

			usedTokens := 0
			for _, file := range sortedByScore {
				content, err := os.ReadFile(file.Path)
				if err != nil || !utf8.Valid(content) {
					continue
				}
				fileTokens := estimator.EstimateText(string(content))
				if usedTokens+fileTokens <= currentBudget {
					file.Tokens = fileTokens
					selectedFiles = append(selectedFiles, file)
					usedTokens += fileTokens
				} else {
					fmt.Printf("  ➖ Skipped (Budget): %s (%d tokens)\n", file.RelPath, fileTokens)
				}
			}
		}

		// Sort selected files alphabetically to ensure deterministic output
		sort.Slice(selectedFiles, func(i, j int) bool {
			return selectedFiles[i].RelPath < selectedFiles[j].RelPath
		})

		// Estimate tokens for selected files
		for i := range selectedFiles {
			content, err := os.ReadFile(selectedFiles[i].Path)
			if err == nil && utf8.Valid(content) {
				selectedFiles[i].Tokens = estimator.EstimateText(string(content))
			}
		}

		// 5. Generate Tree and Body
		var bodyBuilder strings.Builder
		if packTree {
			treeStr := generateTree(selectedFiles)
			bodyBuilder.WriteString("## File Tree\n\n```text\n" + treeStr + "```\n\n")
		}

		bodyBuilder.WriteString("## File Contents\n\n")
		for _, file := range selectedFiles {
			content, err := os.ReadFile(file.Path)
			if err != nil {
				continue
			}
			strContent := string(content)

			if packFormat == "xml" {
				bodyBuilder.WriteString(fmt.Sprintf("<file path=\"%s\">\n%s\n</file>\n\n", file.RelPath, strContent))
			} else {
				ext := strings.TrimPrefix(filepath.Ext(file.RelPath), ".")
				if ext == "" {
					ext = "txt"
				}
				bodyBuilder.WriteString(fmt.Sprintf("## File: %s\n```%s\n%s\n```\n\n", file.RelPath, ext, strContent))
			}
			fmt.Printf("  ➕ Packed: %s\n", file.RelPath)
		}

		// 6. Generate Token Summary Table
		var summaryBuilder strings.Builder
		summaryBuilder.WriteString("## Token Summary\n\n| File | Estimated Tokens |\n|---|---:|\n")
		totalEstimated := 0
		for _, file := range selectedFiles {
			fmt.Fprintf(&summaryBuilder, "| %s | %d |\n", file.RelPath, file.Tokens)
			totalEstimated += file.Tokens
		}
		fmt.Fprintf(&summaryBuilder, "| **Total** | **%d** |\n\n", totalEstimated)

		bodyBuilder.WriteString(summaryBuilder.String())

		// 7. Generate Header
		var headerBuilder strings.Builder
		headerBuilder.WriteString("# CodePicker Context Pack\n\n")

		headerBuilder.WriteString("## Task\n\n")
		if packTask != "" {
			headerBuilder.WriteString(packTask + "\n\n")
		} else {
			headerBuilder.WriteString("<task or empty>\n\n")
		}

		headerBuilder.WriteString("## Repo\n\n")
		headerBuilder.WriteString(cwd + "\n\n")

		headerBuilder.WriteString("## Generated\n\n")
		headerBuilder.WriteString(time.Now().Format("2006-01-02 15:04:05 MST") + "\n\n")

		headerBuilder.WriteString("## Token Estimate\n\n")
		headerBuilder.WriteString(fmt.Sprintf("%d\n\n", totalEstimated))

		headerBuilder.WriteString("## Files Included\n\n")
		headerBuilder.WriteString(fmt.Sprintf("%d\n\n", len(selectedFiles)))

		headerBuilder.WriteString("## Files Excluded\n\n")
		headerBuilder.WriteString(fmt.Sprintf("%d\n\n", excludedCount))

		// Combine all sections
		finalOutput := headerBuilder.String() + bodyBuilder.String()

		// 8. Write to Output file
		if err := os.WriteFile(packOutput, []byte(finalOutput), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

		// 9. Pack Manifest (JSON block at end if --meta)
		if packMeta {
			manifest := PackManifest{
				TotalBytes:      int64(len(finalOutput)),
				FileCount:       len(selectedFiles),
				EstimatedTokens: totalEstimated,
				Mode:            selectedMode,
				GeneratedAt:     time.Now(),
			}
			if err := appendManifest(packOutput, manifest); err != nil {
				fmt.Printf("⚠️  Failed to append manifest: %v\n", err)
			}
		}

		fmt.Printf("\n✅ Pack Complete!\n")
		fmt.Printf("   Mode: %s\n", selectedMode)
		fmt.Printf("   Est. Tokens: ~%d\n", totalEstimated)

		// 10. Copy to Clipboard
		if packClipboard {
			if err := clipboard.WriteAll(finalOutput); err != nil {
				fmt.Printf("⚠️  Failed to copy to clipboard: %v\n", err)
			} else {
				fmt.Printf("📋 Output copied to clipboard!\n")
			}
		}

		// 11. Post-Process Splitting
		if packSplit {
			fmt.Printf("\n🔪 Splitting output into %d token chunks...\n", packSplitTokens)
			if err := splitPackedFile(packOutput, packSplitTokens); err != nil {
				return fmt.Errorf("failed to split packed file: %w", err)
			}
		} else {
			fmt.Printf("   Output: %s\n", packOutput)
		}

		if GetJSON() {
			os.Stdout = origStdout
			packJSON := map[string]interface{}{
				"status":           "pass",
				"output_file":      packOutput,
				"file_count":       len(selectedFiles),
				"estimated_tokens": totalEstimated,
			}
			jsonData, _ := json.MarshalIndent(packJSON, "", "  ")
			fmt.Println(string(jsonData))
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
	packCmd.Flags().BoolVarP(&packClipboard, "clipboard", "c", false, "Copy output to clipboard")
	packCmd.Flags().BoolVarP(&packMeta, "meta", "m", false, "Include metadata, headers and manifest in output")
	packCmd.Flags().StringVar(&packTask, "task", "", "Associated task description")
	packCmd.Flags().BoolVar(&packChanged, "changed", false, "Only pack files that have been modified, added, renamed, or are untracked in Git")
	packCmd.Flags().StringVar(&packProfile, "profile", "", "Preconfigured include/exclude profile (e.g. 'go-cli', 'go-web', 'sveltekit', 'node', 'python', 'fullstack')")

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

// Predefined include/exclude profiles
type Profile struct {
	Include []string
	Exclude []string
}

var profiles = map[string]Profile{
	"go-cli": {
		Include: []string{"*.go", "cmd/**", "internal/**", "pkg/**", "go.mod", "go.sum", "Makefile"},
		Exclude: []string{"vendor/**", "dist/**", "tmp/**"},
	},
	"go-web": {
		Include: []string{"*.go", "cmd/**", "internal/**", "pkg/**", "go.mod", "go.sum", "Makefile", "templates/**", "static/**", "public/**"},
		Exclude: []string{"vendor/**", "dist/**", "tmp/**"},
	},
	"sveltekit": {
		Include: []string{"src/**", "static/**", "package.json", "svelte.config.js", "vite.config.js", "vite.config.ts", "tsconfig.json", "jsconfig.json", "*.html", "*.css", "*.js", "*.ts"},
		Exclude: []string{"node_modules/**", ".svelte-kit/**", "build/**", "dist/**"},
	},
	"node": {
		Include: []string{"src/**", "lib/**", "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "*.js", "*.ts", "*.json"},
		Exclude: []string{"node_modules/**", "dist/**", "build/**"},
	},
	"python": {
		Include: []string{"*.py", "requirements.txt", "pyproject.toml", "poetry.lock", "setup.py", "Pipfile", "Pipfile.lock"},
		Exclude: []string{"**/__pycache__/**", ".venv/**", "venv/**", ".pytest_cache/**", "*.egg-info/**"},
	},
	"fullstack": {
		Include: []string{"*.go", "cmd/**", "internal/**", "pkg/**", "go.mod", "go.sum", "Makefile", "src/**", "public/**", "package.json", "package-lock.json", "*.js", "*.ts", "*.svelte", "*.py"},
		Exclude: []string{"vendor/**", "node_modules/**", "dist/**", "build/**", "tmp/**", ".svelte-kit/**"},
	},
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

func scanTargets(cwd string, targets []string, ignorePatterns []string, changedMap map[string]bool, profile Profile) ([]FileEntry, int64, int, error) {
	var files []FileEntry
	var totalBytes int64
	var excludedCount int
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

			relSlash := filepath.ToSlash(rel)

			// Skip output file and .git immediately
			if info.Name() == packOutput || info.Name() == ".git" || info.Name() == "codepicker_out" {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Check default excludes (junk, logs, binary, archive, etc.)
			if isExcludedByDefault(relSlash) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				excludedCount++
				return nil
			}

			// Check profile matching
			if packProfile != "" && !info.IsDir() {
				if !matchProfile(relSlash, profile) {
					excludedCount++
					return nil
				}
			}

			// Check Git changed matching
			if packChanged && !info.IsDir() {
				if !changedMap[relSlash] {
					excludedCount++
					return nil
				}
			}

			// 1. Check Custom Ignores (.codepickerignore)
			if isIgnored(rel, ignorePatterns) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				excludedCount++
				return nil
			}

			if info.IsDir() {
				return nil
			}

			// 3. Filter Files (Extensions & Whitelist)
			if !shouldPack(path, info) {
				excludedCount++
				return nil
			}

			files = append(files, FileEntry{Path: path, RelPath: rel, Info: info})
			totalBytes += info.Size()

			return nil
		})

		if err != nil {
			return nil, 0, 0, err
		}
	}

	return files, totalBytes, excludedCount, nil
}

func isExcludedByDefault(relPath string) bool {
	parts := strings.Split(relPath, "/")
	for _, part := range parts {
		if part == ".git" ||
			part == "node_modules" ||
			part == "vendor" ||
			part == "dist" ||
			part == "build" ||
			part == "tmp" ||
			part == ".cache" ||
			part == "coverage" {
			return true
		}
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	binaryOrArchiveExtensions := map[string]bool{
		".log": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".mp4": true, ".zip": true, ".tar": true, ".gz": true,
	}
	return binaryOrArchiveExtensions[ext]
}

func matchPattern(path string, pattern string) bool {
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			return strings.HasPrefix(path, parts[0]) && strings.HasSuffix(path, parts[1])
		}
	}

	if strings.Contains(pattern, "*") {
		if !strings.Contains(pattern, "/") {
			matched, _ := filepath.Match(pattern, filepath.Base(path))
			return matched
		}
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	return path == pattern || strings.HasPrefix(path, pattern+"/")
}

func matchProfile(rel string, p Profile) bool {
	matchedInclude := len(p.Include) == 0
	for _, inc := range p.Include {
		if matchPattern(rel, inc) {
			matchedInclude = true
			break
		}
	}
	if !matchedInclude {
		return false
	}
	for _, exc := range p.Exclude {
		if matchPattern(rel, exc) {
			return false
		}
	}
	return true
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
		fmt.Printf("  ⚠️  Could not remove original monolithic file %s: %v\n", inputPath, err)
	} else {
		fmt.Printf("  🧹 Removed original un-split file to save space.\n")
	}

	return nil
}

// --- Helpers ---

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
	type Node struct {
		Name     string
		Children map[string]*Node
		IsFile   bool
	}

	root := &Node{Children: make(map[string]*Node)}

	for _, f := range files {
		parts := strings.Split(filepath.ToSlash(f.RelPath), "/")
		curr := root
		for i, part := range parts {
			if part == "" {
				continue
			}
			isFile := (i == len(parts)-1)
			if _, exists := curr.Children[part]; !exists {
				curr.Children[part] = &Node{
					Name:     part,
					Children: make(map[string]*Node),
					IsFile:   isFile,
				}
			}
			curr = curr.Children[part]
		}
	}

	var sb strings.Builder
	var printNode func(n *Node, indent int)
	printNode = func(n *Node, indent int) {
		var keys []string
		for k := range n.Children {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var sortedKeys []string
		for _, k := range keys {
			if !n.Children[k].IsFile {
				sortedKeys = append(sortedKeys, k)
			}
		}
		for _, k := range keys {
			if n.Children[k].IsFile {
				sortedKeys = append(sortedKeys, k)
			}
		}

		for _, k := range sortedKeys {
			child := n.Children[k]
			spaces := strings.Repeat("  ", indent)
			if child.IsFile {
				sb.WriteString(fmt.Sprintf("%s%s\n", spaces, child.Name))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s/\n", spaces, child.Name))
				printNode(child, indent+1)
			}
		}
	}

	printNode(root, 0)
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
