package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/git"
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
	diffRef     string
)

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
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfgFile *config.ConfigFile
		if configFile != "" {
			var err error
			cfgFile, err = config.LoadConfigFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to load config file: %w", err)
			}
			appLogger.Info(fmt.Sprintf("Loaded config from: %s", configFile))
		} else {
			cfgFile, _ = config.LoadConfigFile("")
			if cfgFile != nil {
				appLogger.Info("Found default config file")
			}
		}

		if cfgFile != nil {
			applyConfig(cmd, cfgFile)
		}

		appLogger.Debug(fmt.Sprintf("Starting with source: %s", srcDir))

		absSrc, err := paths.Sanitize(srcDir)
		if err != nil {
			return fmt.Errorf("invalid source directory: %w", err)
		}

		info, err := os.Stat(absSrc)
		if err != nil {
			return fmt.Errorf("cannot access source directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("source path is not a directory: %s", absSrc)
		}

		if outPath == "" {
			dirName := filepath.Base(absSrc)
			if dirName == "." || dirName == string(filepath.Separator) {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get working directory: %w", err)
				}
				dirName = filepath.Base(wd)
			}
			outPath = fmt.Sprintf("%s_context.md", dirName)
		}

		absOut, err := paths.Sanitize(outPath)
		if err != nil {
			return fmt.Errorf("invalid output path: %w", err)
		}

		if err := paths.ValidateOutput(absOut); err != nil {
			return fmt.Errorf("output validation failed: %w", err)
		}

		if filepath.Ext(absOut) == "" {
			absOut += ".md"
		}

		if absSrc == absOut {
			return fmt.Errorf("cannot write context to source directory root")
		}

		// FIX: Pass 'showTokens' to the writer so it knows whether to run the tokenizer
		w := writer.NewConcatStrategy(absOut, minify, showTokens)

		if err := runScan(cmd.Context(), w, absSrc, cmd); err != nil {
			return err
		}

		appLogger.Info(fmt.Sprintf("Output: %s", absOut))
		if showTokens {
			appLogger.Info(fmt.Sprintf("Token Count: %d", w.TokenCount))
		}
		return nil
	},
}

func Execute() {
	appLogger = logger.NewStandardLogger(1)
	if err := rootCmd.Execute(); err != nil {
		appLogger.Error(fmt.Sprintf("Fatal error: %v", err))
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&srcDir, "src", "s", ".", "Source directory to scan")
	rootCmd.Flags().StringVarP(&outPath, "out", "o", "", "Output file path (default: [dir_name]_context.md)")
	rootCmd.Flags().BoolVarP(&showTokens, "tokens", "t", false, "Show precise token count (BPE)")
	rootCmd.Flags().BoolVarP(&minify, "minify", "m", true, "Remove comments and extra whitespace")
	rootCmd.Flags().StringVarP(&includeExts, "include", "i", "", "Comma-separated extensions to include")
	rootCmd.Flags().StringVarP(&ignoreDirs, "exclude", "e", "", "Comma-separated directories to exclude")
	rootCmd.Flags().StringVarP(&diffRef, "diff", "d", "", "Scan only changed files (e.g. 'main', 'HEAD~1', or empty for staged/unstaged)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path (default: .codepicker.yml)")
}

func applyConfig(cmd *cobra.Command, cfgFile *config.ConfigFile) {
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
		appLogger = logger.NewStandardLogger(2)
	}
	if len(cfgFile.Include) > 0 && includeExts == "" {
		includeExts = strings.Join(cfgFile.Include, ",")
	}
	if len(cfgFile.Exclude) > 0 && ignoreDirs == "" {
		ignoreDirs = strings.Join(cfgFile.Exclude, ",")
	}
}

func runScan(ctx context.Context, w writer.OutputStrategy, absSrc string, cmd *cobra.Command) error {
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

	hasDiff := false
	if cmd.Flags().Lookup("diff") != nil {
		hasDiff = flagChanged(cmd.Flags(), "diff") || diffRef != ""
	}

	if w.Name() != "Tree" {
		appLogger.Info(fmt.Sprintf("Scanning: %s", absSrc))
		if hasDiff {
			appLogger.Info(fmt.Sprintf("Diff Mode: %s", diffRef))
		}
		if includeExts != "" {
			appLogger.Info(fmt.Sprintf("Including: %s", includeExts))
		}
		if minify {
			appLogger.Info("Minification enabled (AST-based)")
		}
	}

	s := scanner.NewScanner(absSrc, w, cfg, appLogger)

	if hasDiff {
		files, err := git.GetChangedFiles(diffRef)
		if err != nil {
			return fmt.Errorf("diff mode failed: %w", err)
		}
		if len(files) == 0 {
			appLogger.Warn("No changed files found via git diff.")
			return nil
		}
		appLogger.Info(fmt.Sprintf("Restricting scan to %d changed files", len(files)))
		s.SetWhitelist(files)
	}

	if err := s.Scan(ctx); err != nil {
		appLogger.Error(fmt.Sprintf("Scan failed: %v", err))
		return err
	}

	if w.Name() != "Tree" {
		elapsed := time.Since(start)
		appLogger.Debug(fmt.Sprintf("Scan completed in %v", elapsed))
	}
	return nil
}

func flagChanged(flags interface{ Changed(string) bool }, name string) bool {
	return flags.Changed(name)
}
