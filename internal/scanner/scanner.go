package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/writer"
	ignore "github.com/sabhiram/go-gitignore"
)

type Scanner struct {
	Root         string
	Writer       writer.OutputStrategy
	Config       *config.Config
	GitIgnore    *ignore.GitIgnore
	CustomIgnore *ignore.GitIgnore
	Logger       logger.Logger
	Whitelist    map[string]bool
}

func NewScanner(root string, w writer.OutputStrategy, cfg *config.Config, log logger.Logger) *Scanner {
	s := &Scanner{
		Root:   root,
		Writer: w,
		Config: cfg,
		Logger: log,
	}

	if err := s.Writer.Init(); err != nil {
		s.Logger.Error(fmt.Sprintf("Failed to init writer: %v", err))
	}

	gitIgnorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(gitIgnorePath)
		s.GitIgnore = ign
		s.Logger.Debug("Loaded .gitignore")
	}

	cpIgnorePath := filepath.Join(root, ".codepickerignore")
	if _, err := os.Stat(cpIgnorePath); err == nil {
		ign, _ := ignore.CompileIgnoreFile(cpIgnorePath)
		s.CustomIgnore = ign
		s.Logger.Debug("Loaded .codepickerignore")
	}

	return s
}

func (s *Scanner) SetWhitelist(files map[string]bool) {
	s.Whitelist = files
}

type scanJob struct {
	absPath string
	relPath string
}

type scanResult struct {
	path string
	err  error
}

func (s *Scanner) Scan(ctx context.Context) error {
	// 1. Setup Channels
	jobs := make(chan scanJob, 100)
	results := make(chan scanResult, 100)
	var wg sync.WaitGroup

	// 2. Start Workers
	// The Writer strategy is now responsible for thread-safety internally.
	workerCount := runtime.NumCPU()
	s.Logger.Debug(fmt.Sprintf("Starting scan with %d workers", workerCount))

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if s.Writer.Name() != "Tree" {
					s.Logger.Info(fmt.Sprintf("Picked: %s", job.relPath))
				}

				// Direct write call - Writer handles the locking
				err := s.Writer.Write(job.absPath, job.relPath)

				results <- scanResult{path: job.relPath, err: err}
			}
		}(i)
	}

	// 3. Start Walker
	go func() {
		defer close(jobs)

		err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
			select {
			case <-ctx.Done():
				return filepath.SkipAll
			default:
			}

			if err != nil {
				if path == s.Root {
					return err
				}
				s.Logger.Warn(fmt.Sprintf("Skipping %s: access denied or error: %v", path, err))
				return filepath.SkipDir
			}

			relPath, relErr := filepath.Rel(s.Root, path)
			if relErr != nil {
				return nil
			}
			if relPath == "." {
				return nil
			}

			if s.Writer.ShouldSkip(path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			cleanRel := filepath.ToSlash(relPath)

			if s.Whitelist != nil {
				if !d.IsDir() {
					if !s.Whitelist[cleanRel] {
						return nil
					}
				}
			}

			if s.GitIgnore != nil && s.GitIgnore.MatchesPath(relPath) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if s.CustomIgnore != nil && s.CustomIgnore.MatchesPath(relPath) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return filepath.SkipDir
				}
				if s.Config.IgnoredDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			name := strings.ToLower(d.Name())

			if !s.Config.AllowedExts[ext] && !config.IsSpecialFile(name) {
				return nil
			}

			jobs <- scanJob{absPath: path, relPath: relPath}
			return nil
		})

		if err != nil {
			s.Logger.Error(fmt.Sprintf("Walk failed: %v", err))
		}
	}()

	// 4. Wait and Cleanup
	go func() {
		wg.Wait()
		close(results)
	}()

	// 5. Monitor results
	var scanErrors []error
	count := 0
	for res := range results {
		if res.err != nil {
			s.Logger.Warn(fmt.Sprintf("Failed to process %s: %v", res.path, res.err))
			scanErrors = append(scanErrors, res.err)
		} else {
			count++
		}
	}

	if len(scanErrors) > 0 {
		return fmt.Errorf("scan completed with %d errors", len(scanErrors))
	}

	return nil
}
