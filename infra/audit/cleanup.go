package audit

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CleanupPolicy defines rules for removing old audit logs.
type CleanupPolicy struct {
	MaxAge   time.Duration // Delete files older than this
	MaxCount int           // Keep only N most recent
}

// CleanupAudits scans the audit directory and applies retention rules.
func CleanupAudits(dir string, policy CleanupPolicy) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type auditFile struct {
		path string
		time time.Time
	}

	var files []auditFile
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Rule 1: Max Age
		if policy.MaxAge > 0 && now.Sub(info.ModTime()) > policy.MaxAge {
			os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}

		files = append(files, auditFile{
			path: filepath.Join(dir, entry.Name()),
			time: info.ModTime(),
		})
	}

	// Rule 2: Max Count
	if policy.MaxCount > 0 && len(files) > policy.MaxCount {
		// Sort by time (oldest first)
		sort.Slice(files, func(i, j int) bool {
			return files[i].time.Before(files[j].time)
		})

		toDelete := len(files) - policy.MaxCount
		for i := 0; i < toDelete; i++ {
			os.Remove(files[i].path)
		}
	}

	return nil
}
