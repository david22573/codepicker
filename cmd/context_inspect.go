package cmd

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/database"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect relevance scores for files in working memory",
	Long: `Display detailed relevance scoring information for all files currently loaded in working memory.
	
This command shows how the intelligent context eviction system scores each file based on:
- Recency: How recently the file was accessed
- Frequency: How often the file has been accessed
- Importance: Inherent importance based on file type and path
- Relationships: Connections to other files in memory

Files with lower scores are more likely to be evicted when context limits are reached.`,
	RunE: runInspect,
}

func init() {
	contextCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	storageDir, err := getStorageDir()
	if err != nil {
		return fmt.Errorf("failed to get storage directory: %w", err)
	}

	store, err := database.New(storageDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Check if there are any files in memory
	files, err := store.ListMemoryFiles()
	if err != nil {
		return fmt.Errorf("failed to list memory files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No files currently in working memory.")
		fmt.Println("\nUse 'codepicker context add <file>' to add files to memory.")
		return nil
	}

	fmt.Printf("📊 Analyzing %d files in working memory...\n\n", len(files))

	scorer := store.GetRelevanceScorer()
	output, err := scorer.DebugScores()
	if err != nil {
		return fmt.Errorf("failed to calculate relevance scores: %w", err)
	}

	fmt.Println(output)

	// Show summary statistics
	scores, err := scorer.CalculateRelevanceScores()
	if err != nil {
		return err
	}

	if len(scores) > 0 {
		totalTokens := 0
		var minScore, maxScore float64 = 1.0, 0.0
		var totalScore float64

		for _, s := range scores {
			totalTokens += s.TokenCount
			totalScore += s.Score
			if s.Score < minScore {
				minScore = s.Score
			}
			if s.Score > maxScore {
				maxScore = s.Score
			}
		}

		avgScore := totalScore / float64(len(scores))

		fmt.Println("\n=== SUMMARY ===")
		fmt.Printf("Total Files:    %d\n", len(scores))
		fmt.Printf("Total Tokens:   %d / %d (%.1f%% of max)\n",
			totalTokens, database.MaxContextTokens,
			float64(totalTokens)/float64(database.MaxContextTokens)*100)
		fmt.Printf("Score Range:    %.3f - %.3f\n", minScore, maxScore)
		fmt.Printf("Average Score:  %.3f\n", avgScore)

		if totalTokens > database.MaxContextTokens {
			excess := totalTokens - database.MaxContextTokens
			fmt.Printf("\n⚠️  WARNING: Context exceeds limit by %d tokens!\n", excess)
			fmt.Println("Files with lowest scores will be evicted on next access.")
		}
	}

	return nil
}

func getStorageDir() (string, error) {
	// Try to get storage dir from command flag or environment
	// For now, use default
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir + "/.codepicker/storage", nil
}
