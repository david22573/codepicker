package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/paths"
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

// appLogger is shared by subcommands in the cmd package
var appLogger logger.Logger

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "Harvest code for AI consumption",
	Long:  `Scans a directory and combines code files into a single context file.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := 1
		if verbose {
			level = 2
		}
		appLogger = logger.NewStandardLogger(level)
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Config Loading
		var cfgFile *config.ConfigFile
		if configFile != "" {
			var err error
			cfgFile, err = config.LoadConfigFile(configFile)
			if err != nil {
				appLogger.Error(fmt.Sprintf("Failed to load config file: %v", err))
				os.Exit(1)
			}
			appLogger.Info(fmt.Sprintf("Loaded config from: %s", configFile))
		} else {
			cfgFile, _ = config.LoadConfigFile("")
			if cfgFile != nil {
				appLogger.Info("Found default config file")
			}
		}

		// Apply Config
		if cfgFile != nil {
			if srcDir == "." && cfgFile.Src != "" {
				srcDir = cfgFile.Src
			}
			if outPath == "" && cfgFile.Output != "" {
				outPath = cfgFile.Output
			}
			if !cmd.Flags().Changed("minify") && cfgFile.Minify {
				minify = cfgFile.Minify
			}
			if !cmd.Flags().Changed("tokens") && cfgFile.Tokens {
				showTokens = cfgFile.Tokens
			}
			if cfgFile.Verbose {
				appLogger = logger.NewStandardLogger(2) // Upgrade logger
			}
			if len(cfgFile.Include) > 0 && includeExts == "" {
				includeExts = strings.Join(cfgFile.Include, ",")
			}
			if len(cfgFile.Exclude) > 0 && ignoreDirs == "" {
				ignoreDirs = strings.Join(cfgFile.Exclude, ",")
			}
			if askModel == "" && cfgFile.AI.Model != "" && cmd.Name() == "ask" {
				askModel = cfgFile.AI.Model
			}
		}

		appLogger.Debug(fmt.Sprintf("Starting with source: %s", srcDir))

		// Validation using new packages
		absSrc, err := paths.Sanitize(srcDir)
		if err != nil {
			appLogger.Error(fmt.Sprintf("Invalid source directory: %v", err))
			os.Exit(1)
		}

		info, err := os.Stat(absSrc)
		if err != nil {
			appLogger.Error(fmt.Sprintf("Cannot access source directory: %v", err))
			os.Exit(1)
		}
		if !info.IsDir() {
			appLogger.Error(fmt.Sprintf("Source path is not a directory: %s", absSrc))
			os.Exit(1)
		}

		if outPath == "" {
			dirName := filepath.Base(absSrc)
			if dirName == "." || dirName == string(filepath.Separator) {
				wd, err := os.Getwd()
				if err != nil {
					appLogger.Error(fmt.Sprintf("Failed to get working directory: %v", err))
					os.Exit(1)
				}
				dirName = filepath.Base(wd)
			}
			outPath = fmt.Sprintf("%s_context.md", dirName)
		}

		absOut, err := paths.Sanitize(outPath)
		if err != nil {
			appLogger.Error(fmt.Sprintf("Invalid output path: %v", err))
			os.Exit(1)
		}

		if err := paths.ValidateOutput(absOut); err != nil {
			appLogger.Error(fmt.Sprintf("Output validation failed: %v", err))
			os.Exit(1)
		}

		if filepath.Ext(absOut) == "" {
			absOut += ".md"
		}

		if absSrc == absOut {
			appLogger.Error("Cannot write context to source directory root")
			os.Exit(1)
		}

		w := writer.NewConcatStrategy(absOut, minify)
		runScan(cmd.Context(), w, absSrc)

		fmt.Printf("📦 Output: %s\n", absOut)
		if showTokens {
			fmt.Printf("🔢 Estimated Tokens: ~%d\n", w.TokenEstimate)
		}
	},
}

func Execute() {
	// Initialize default logger to avoid nil pointer before Run
	appLogger = logger.NewStandardLogger(1)
	if err := rootCmd.Execute(); err != nil {
		appLogger.Error(fmt.Sprintf("Fatal error: %v", err))
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

// runScan is shared by subcommands
func runScan(ctx context.Context, w writer.OutputStrategy, absSrc string) {
	start := time.Now()
	appLogger.Info(fmt.Sprintf("Scanning directory: %s", absSrc))

	cfg := config.NewConfig()
	if includeExts != "" {
		exts := strings.Split(includeExts, ",")
		cfg.AddAllowedExtensions(exts)
		appLogger.Debug(fmt.Sprintf("Including extensions: %v", exts))
	}

	if ignoreDirs != "" {
		dirs := strings.Split(ignoreDirs, ",")
		cfg.AddIgnoredDirs(dirs)
		appLogger.Debug(fmt.Sprintf("Excluding directories: %v", dirs))
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

	// Inject appLogger into scanner
	s := scanner.NewScanner(absSrc, w, cfg, appLogger)

	if err := s.Scan(ctx); err != nil {
		appLogger.Error(fmt.Sprintf("Scan failed: %v", err))
		os.Exit(1)
	}

	if w.Name() != "Tree" {
		elapsed := time.Since(start)
		fmt.Printf("✅ Done in %v\n", elapsed)
		appLogger.Debug(fmt.Sprintf("Scan completed in %v", elapsed))
	}
}
