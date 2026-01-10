package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/errors"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/spf13/cobra"
)

var (
	srcDir      string
	outPath     string
	showTokens  bool
	minify      bool
	includeExts string
	ignoreDirs  string
	configFile  string
	verbose     bool
)

var logger = log.New(os.Stderr, "", 0)
var logLevel = 1

func logInfo(msg string) {
	if logLevel >= 1 {
		logger.Printf("ℹ️  %s", msg)
	}
}

func logWarn(msg string) {
	if logLevel >= 1 {
		logger.Printf("⚠️  %s", msg)
	}
}

func logDebug(msg string) {
	if logLevel >= 2 {
		logger.Printf("🔧 %s", msg)
	}
}

func logError(msg string) {
	logger.Printf("❌ %s", msg)
}

func sanitizePath(path string) (string, error) {
	clean := filepath.Clean(path)

	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", &errors.ValidationError{
			Field:   "path",
			Message: "path escapes working directory",
			Value:   path,
		}
	}

	return abs, nil
}

func validateOutPath(out string) error {
	forbidden := []string{"/", "/usr", "/etc", "/bin", "/sbin", "/opt", "/sys", "/proc", "/dev"}
	for _, forbiddenDir := range forbidden {
		if out == forbiddenDir || strings.HasPrefix(out, forbiddenDir+string(filepath.Separator)) {
			return &errors.ValidationError{
				Field:   "outPath",
				Message: "forbidden system directory",
				Value:   out,
			}
		}
	}

	base := filepath.Base(out)
	if base == "go.mod" || base == "go.sum" || base == "package.json" || base == "package-lock.json" {
		return &errors.ValidationError{
			Field:   "outPath",
			Message: "refusing to overwrite important file",
			Value:   out,
		}
	}

	return nil
}

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "Harvest code for AI consumption",
	Long:  `Scans a directory and combines code files into a single context file.`,
	Run: func(cmd *cobra.Command, args []string) {
		if verbose {
			logLevel = 2
		}

		var cfgFile *config.ConfigFile
		if configFile != "" {
			var err error
			cfgFile, err = config.LoadConfigFile(configFile)
			if err != nil {
				logError(fmt.Sprintf("Failed to load config file: %v", err))
				os.Exit(1)
			}
			logInfo(fmt.Sprintf("Loaded config from: %s", configFile))
		} else {
			cfgFile, _ = config.LoadConfigFile("")
			if cfgFile != nil {
				logInfo("Found default config file")
			}
		}

		if cfgFile != nil {
			if srcDir == "." && cfgFile.Src != "" {
				srcDir = cfgFile.Src
				logDebug(fmt.Sprintf("Config: src = %s", srcDir))
			}
			if outPath == "" && cfgFile.Output != "" {
				outPath = cfgFile.Output
				logDebug(fmt.Sprintf("Config: output = %s", outPath))
			}
			if !cmd.Flags().Changed("minify") && cfgFile.Minify {
				minify = cfgFile.Minify
				logDebug("Config: minify = true")
			}
			if !cmd.Flags().Changed("tokens") && cfgFile.Tokens {
				showTokens = cfgFile.Tokens
				logDebug("Config: tokens = true")
			}
			if cfgFile.Verbose {
				logLevel = 2
				logDebug("Config: verbose = true")
			}
			if len(cfgFile.Include) > 0 && includeExts == "" {
				includeExts = strings.Join(cfgFile.Include, ",")
				logDebug(fmt.Sprintf("Config: include = %v", cfgFile.Include))
			}
			if len(cfgFile.Exclude) > 0 && ignoreDirs == "" {
				ignoreDirs = strings.Join(cfgFile.Exclude, ",")
				logDebug(fmt.Sprintf("Config: exclude = %v", cfgFile.Exclude))
			}
			if askModel == "" && cfgFile.AI.Model != "" && cmd.Name() == "ask" {
				askModel = cfgFile.AI.Model
				logDebug(fmt.Sprintf("Config: ai.model = %s", askModel))
			}
		}

		logDebug(fmt.Sprintf("Starting with source: %s", srcDir))

		absSrc, err := sanitizePath(srcDir)
		if err != nil {
			logError(fmt.Sprintf("Invalid source directory: %v", err))
			os.Exit(1)
		}

		info, err := os.Stat(absSrc)
		if err != nil {
			logError(fmt.Sprintf("Cannot access source directory: %v", err))
			os.Exit(1)
		}
		if !info.IsDir() {
			logError(fmt.Sprintf("Source path is not a directory: %s", absSrc))
			os.Exit(1)
		}

		if outPath == "" {
			dirName := filepath.Base(absSrc)
			if dirName == "." || dirName == string(filepath.Separator) {
				wd, err := os.Getwd()
				if err != nil {
					logError(fmt.Sprintf("Failed to get working directory: %v", err))
					os.Exit(1)
				}
				dirName = filepath.Base(wd)
			}
			outPath = fmt.Sprintf("%s_context.md", dirName)
			logDebug(fmt.Sprintf("Default output path: %s", outPath))
		}

		absOut, err := sanitizePath(outPath)
		if err != nil {
			logError(fmt.Sprintf("Invalid output path: %v", err))
			os.Exit(1)
		}

		if err := validateOutPath(absOut); err != nil {
			logError(fmt.Sprintf("Output validation failed: %v", err))
			os.Exit(1)
		}

		if filepath.Ext(absOut) == "" {
			absOut += ".md"
		}

		parentDir := filepath.Dir(absOut)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			logError(fmt.Sprintf("Cannot create output directory: %v", err))
			os.Exit(1)
		}

		if absSrc == absOut {
			logError("Cannot write context to source directory root")
			os.Exit(1)
		}

		w := writer.NewConcatStrategy(absOut, minify)
		// FIXED: Pass cmd.Context()
		runScan(cmd.Context(), w, absSrc)

		fmt.Printf("📦 Output: %s\n", absOut)
		if showTokens {
			fmt.Printf("🔢 Estimated Tokens: ~%d\n", w.TokenEstimate)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logError(fmt.Sprintf("Fatal error: %v", err))
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory to scan")
	rootCmd.Flags().StringVarP(&outPath, "out", "o", "", "Output file path (default: [dir_name]_context.md)")
	rootCmd.Flags().BoolVarP(&showTokens, "tokens", "t", false, "Show estimated token count")
	rootCmd.Flags().BoolVarP(&minify, "minify", "m", true, "Remove comments and extra whitespace to save tokens")
	rootCmd.Flags().StringVarP(&includeExts, "include", "i", "", "Comma-separated extensions to include (e.g. .vue,.svelte)")
	rootCmd.Flags().StringVarP(&ignoreDirs, "exclude", "e", "", "Comma-separated directories to exclude")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path (default: .codepicker.yml)")
}

// FIXED: Added context parameter
func runScan(ctx context.Context, w writer.OutputStrategy, absSrc string) {
	start := time.Now()
	logInfo(fmt.Sprintf("Scanning directory: %s", absSrc))

	cfg := config.NewConfig()
	if includeExts != "" {
		exts := strings.Split(includeExts, ",")
		cfg.AddAllowedExtensions(exts)
		logDebug(fmt.Sprintf("Including extensions: %v", exts))
	}

	if ignoreDirs != "" {
		dirs := strings.Split(ignoreDirs, ",")
		cfg.AddIgnoredDirs(dirs)
		logDebug(fmt.Sprintf("Excluding directories: %v", dirs))
	}

	if w.Name() != "Tree" {
		fmt.Printf("🚇 Scanning: %s\n", absSrc)
		if includeExts != "" {
			fmt.Printf("➕ Including: %s\n", includeExts)
		}
		if minify {
			fmt.Println("✂️  Minification enabled (AST-based)")
		}
	}

	s := scanner.NewScanner(absSrc, w, cfg)
	// FIXED: Pass context to Scan
	if err := s.Scan(ctx); err != nil {
		logError(fmt.Sprintf("Scan failed: %v", err))
		os.Exit(1)
	}

	if w.Name() != "Tree" {
		elapsed := time.Since(start)
		fmt.Printf("✅ Done in %v\n", elapsed)
		logDebug(fmt.Sprintf("Scan completed in %v", elapsed))
	}
}

