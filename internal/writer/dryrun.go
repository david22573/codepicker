package writer

import (
	"fmt"

	"github.com/david22573/codepicker/internal/logger"
)

type DryRunStrategy struct {
	wrapped OutputStrategy
	logger  logger.Logger
}

func NewDryRunStrategy(wrapped OutputStrategy, log logger.Logger) *DryRunStrategy {
	return &DryRunStrategy{
		wrapped: wrapped,
		logger:  log,
	}
}

func (d *DryRunStrategy) Init() error {
	d.logger.Info("[DryRun] Initializing writer strategy: " + d.wrapped.Name())
	// We do NOT call wrapped.Init() to ensure no files/directories are created
	return nil
}

func (d *DryRunStrategy) Write(absPath, relPath string) error {
	d.logger.Info(fmt.Sprintf("[DryRun] Would write: %s", relPath))
	return nil
}

func (d *DryRunStrategy) Close() error {
	d.logger.Info("[DryRun] Closing writer")
	return nil
}

func (d *DryRunStrategy) ShouldSkip(path string) bool {
	return d.wrapped.ShouldSkip(path)
}

func (d *DryRunStrategy) Name() string {
	return fmt.Sprintf("DryRun(%s)", d.wrapped.Name())
}
