package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/paths"
)

func validateAPIKey() (string, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")

	if apiKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable is not set")
	}

	if len(apiKey) < constants.MinAPIKeyLength {
		return "", fmt.Errorf("API key appears invalid (length < %d)", constants.MinAPIKeyLength)
	}

	return apiKey, nil
}

func validateFocusFiles(focusList string) ([]string, error) {
	if focusList == "" {
		return nil, nil
	}

	files := strings.Split(focusList, ",")
	var validated []string

	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		clean, err := paths.Sanitize(f)
		if err != nil {
			appLogger.Warn(fmt.Sprintf("Invalid focus file path '%s': %v (skipping)", f, err))
			continue
		}

		// Hard Safety Check
		if strings.Contains(clean, "/.git/") || strings.HasSuffix(clean, "/.git") {
			appLogger.Warn(fmt.Sprintf("Refusing to focus on git internal file: %s", clean))
			continue
		}

		info, err := os.Stat(clean)
		if err != nil {
			appLogger.Warn(fmt.Sprintf("Focus file not found (skipping): %s", clean))
			continue
		}

		if info.IsDir() {
			appLogger.Warn(fmt.Sprintf("Focus path is a directory (skipping, use source dir for full scan): %s", clean))
			continue
		}

		validated = append(validated, clean)
		appLogger.Debug(fmt.Sprintf("Validated focus file: %s", clean))
	}

	return validated, nil
}

