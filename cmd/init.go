package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"encoding/json"

	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/domain/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CodePicker in the current directory",
	Long:  `Scaffolds the necessary .codepicker directory, configuration files, and security policies required to run the agent.`,
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := os.Getwd()
		configDir := filepath.Join(cwd, ".codepicker")

		fmt.Printf("🚀 Initializing CodePicker in %s...\n", cwd)

		// 1. Create Directories
		dirs := []string{
			configDir,
			filepath.Join(configDir, "logs"),
			filepath.Join(configDir, "shadow"),
			filepath.Join(configDir, "audit"),
			filepath.Join(configDir, "runs"),
		}

		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Printf("❌ Failed to create directory %s: %v\n", dir, err)
				return
			}
		}
		fmt.Println("✅ Created directory structure (.codepicker/)")

		// 2. Create Default Config (codepicker.yaml)
		// We'll place it in the root for visibility, or .codepicker if preferred.
		// Standard practice is often a dotfile or inside the config dir.
		configPath := filepath.Join(cwd, "codepicker.yaml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			defaultCfg := config.DefaultConfig()
			data, _ := yaml.Marshal(defaultCfg)
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				fmt.Printf("❌ Failed to create config file: %v\n", err)
			} else {
				fmt.Println("✅ Created default configuration (codepicker.yaml)")
			}
		} else {
			fmt.Println("ℹ️  Configuration file already exists, skipping.")
		}

		// 3. Create Default Policy (policy.json)
		policyPath := filepath.Join(cwd, "policy.json")
		if _, err := os.Stat(policyPath); os.IsNotExist(err) {
			defaultPolicy := policy.DefaultPolicy()
			data, _ := json.MarshalIndent(defaultPolicy, "", "  ")
			if err := os.WriteFile(policyPath, data, 0644); err != nil {
				fmt.Printf("❌ Failed to create policy file: %v\n", err)
			} else {
				fmt.Println("✅ Created security policy (policy.json)")
			}
		} else {
			fmt.Println("ℹ️  Policy file already exists, skipping.")
		}

		// 4. Create .gitignore (Critical to not commit shadow files)
		gitignorePath := filepath.Join(cwd, ".gitignore")
		ignoreContent := "\n# CodePicker\n.codepicker/\ncodepicker.yaml\npolicy.json\n"

		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("⚠️  Could not update .gitignore: %v\n", err)
		} else {
			defer f.Close()
			if _, err := f.WriteString(ignoreContent); err == nil {
				fmt.Println("✅ Updated .gitignore")
			}
		}

		fmt.Println(color.GreenString("\n🎉 Initialization Complete!"))
		fmt.Println("You can now run: codepicker run \"Refactor main.go to use a structured logger\"")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
