package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/spf13/cobra"
)

// FIX: Define the flag variable that was missing
var forceInit bool

type InitOptions struct {
	Language    string
	ExtraDirs   string
	EnableMCP   bool
	ModelChoice string
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive project setup wizard",
	RunE: func(cmd *cobra.Command, args []string) error {
		// FIX: Use forceInit check here
		if _, err := os.Stat(".codepicker.yml"); err == nil && !forceInit {
			return fmt.Errorf(".codepicker.yml already exists. Use --force to overwrite")
		}

		opts := InitOptions{
			ModelChoice: constants.DefaultModel,
		}

		// 1. Define the Form
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Primary Language").
					Description("We'll configure ignore patterns and extensions for you.").
					Options(
						huh.NewOption("Go", "go"),
						huh.NewOption("TypeScript/JS", "ts"),
						huh.NewOption("Python", "py"),
						huh.NewOption("Rust", "rs"),
						huh.NewOption("Other (Generic)", "other"),
					).
					Value(&opts.Language),

				huh.NewSelect[string]().
					Title("AI Model").
					Options(
						huh.NewOption("DeepSeek V3 (Recommended)", "deepseek/deepseek-chat"),
						huh.NewOption("Claude 3.5 Sonnet", "anthropic/claude-3.5-sonnet"),
						huh.NewOption("GPT-4o", "openai/gpt-4o"),
					).
					Value(&opts.ModelChoice),

				huh.NewConfirm().
					Title("Enable GitHub MCP?").
					Description("Allows the agent to read GitHub Issues/PRs directly.").
					Value(&opts.EnableMCP),
			),
		)

		// 2. Run the Form
		err := form.Run()
		if err != nil {
			return nil // User cancelled
		}

		// 3. Generate Config based on answers
		configContent := generateConfigContent(opts)

		// 4. Write File
		if err := os.WriteFile(".codepicker.yml", []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		fmt.Println("\n✨ Configuration saved to .codepicker.yml")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	// FIX: Register the force flag
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "Overwrite existing config file")
}

func generateConfigContent(opts InitOptions) string {
	base := fmt.Sprintf(`# .codepicker.yml
src: .
output: codepicker_context.md
ai:
  model: %s
  temperature: 0.0

include:
%s

exclude:
  - .git
  - .codepicker
  - node_modules
  - vendor
`, opts.ModelChoice, getExtensions(opts.Language))

	if opts.EnableMCP {
		base += `
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      - "GITHUB_PERSONAL_ACCESS_TOKEN=YOUR_TOKEN_HERE"
`
	}

	return base
}

func getExtensions(lang string) string {
	switch lang {
	case "go":
		return "  - .go\n  - .mod"
	case "ts":
		return "  - .ts\n  - .tsx\n  - .js\n  - .json"
	case "py":
		return "  - .py\n  - .txt"
	case "rs":
		return "  - .rs\n  - .toml"
	default:
		return "  - .md\n  - .txt"
	}
}
