package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/david22573/codepicker/internal/database"
)

// This example demonstrates the intelligent context eviction system
func main() {
	// Create temporary storage for demo
	tmpDir, err := os.MkdirTemp("", "codepicker-demo-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("=== CodePicker Context Eviction Demo ===\n")

	// Initialize store
	store, err := database.New(tmpDir)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	// Simulate an agent session
	fmt.Println("Phase 1: Initial file loading")
	fmt.Println("--------------------------------")

	files := []struct {
		path    string
		content string
		desc    string
	}{
		{
			path:    "go.mod",
			content: generateContent(50),
			desc:    "Critical config file",
		},
		{
			path:    "internal/database/store.go",
			content: generateContent(1000),
			desc:    "Core implementation",
		},
		{
			path:    "internal/database/store_test.go",
			content: generateContent(800),
			desc:    "Test file (lower priority)",
		},
		{
			path:    "vendor/github.com/external/lib.go",
			content: generateContent(500),
			desc:    "Vendor file (lowest priority)",
		},
		{
			path:    "cmd/main.go",
			content: generateContent(600),
			desc:    "Entry point (high importance)",
		},
	}

	for _, f := range files {
		fmt.Printf("  Adding: %-40s (%s)\n", f.path, f.desc)
		if err := store.UpdateWorkingMemory(f.path, f.content); err != nil {
			log.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond) // Simulate time passing
	}

	// Show initial state
	fmt.Println("\nPhase 2: Initial relevance scores")
	fmt.Println("--------------------------------")
	showScores(store)

	// Simulate working session - repeatedly access core files
	fmt.Println("\nPhase 3: Simulating work session (accessing core files)")
	fmt.Println("--------------------------------")

	for i := 0; i < 5; i++ {
		fmt.Printf("  Access %d: Reading store.go and main.go\n", i+1)
		store.UpdateWorkingMemory("internal/database/store.go", files[1].content)
		store.UpdateWorkingMemory("cmd/main.go", files[4].content)
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("\nPhase 4: Updated relevance scores (after repeated access)")
	fmt.Println("--------------------------------")
	showScores(store)

	// Add many more files to trigger eviction
	fmt.Println("\nPhase 5: Adding more files to trigger eviction")
	fmt.Println("--------------------------------")

	for i := 0; i < 30; i++ {
		path := fmt.Sprintf("internal/utils/helper%d.go", i)
		content := generateContent(3000) // Large files
		fmt.Printf("  Adding: %s (%d tokens)\n", path, len(content)/4)
		if err := store.UpdateWorkingMemory(path, content); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\nPhase 6: Post-eviction state")
	fmt.Println("--------------------------------")
	showScores(store)

	// Show which files survived
	fmt.Println("\nPhase 7: Survival analysis")
	fmt.Println("--------------------------------")

	scorer := store.GetRelevanceScorer()
	scores, _ := scorer.CalculateRelevanceScores()

	fmt.Println("\n✅ Files that survived eviction:")
	for _, s := range scores {
		if s.Path == "go.mod" || s.Path == "internal/database/store.go" ||
			s.Path == "cmd/main.go" || s.Path == "internal/database/store_test.go" ||
			s.Path == "vendor/github.com/external/lib.go" {
			fmt.Printf("  • %-40s (score: %.3f)\n", s.Path, s.Score)
		}
	}

	fmt.Println("\n❌ Files that were evicted:")
	originalFiles := map[string]bool{
		"go.mod":                            false,
		"internal/database/store.go":        false,
		"internal/database/store_test.go":   false,
		"vendor/github.com/external/lib.go": false,
		"cmd/main.go":                       false,
	}

	for _, s := range scores {
		if _, exists := originalFiles[s.Path]; exists {
			originalFiles[s.Path] = true
		}
	}

	for path, survived := range originalFiles {
		if !survived {
			fmt.Printf("  • %s\n", path)
		}
	}

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("\nKey Observations:")
	fmt.Println("  1. Critical files (go.mod) have maximum importance and survive")
	fmt.Println("  2. Frequently accessed files (store.go, main.go) survive due to high frequency")
	fmt.Println("  3. Vendor files and tests are first to be evicted (low importance)")
	fmt.Println("  4. Recent additions with low importance are evicted over critical old files")
}

func showScores(store *database.Store) {
	scorer := store.GetRelevanceScorer()
	scores, err := scorer.CalculateRelevanceScores()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n%-40s | %6s | %6s | %6s | %6s | %6s | %7s\n",
		"File", "Final", "Recency", "Freq", "Import", "Relat", "Access")
	fmt.Println("----------------------------------------------------------------------------------------------------")

	for _, s := range scores {
		// Only show original demo files
		if s.Path == "go.mod" || s.Path == "internal/database/store.go" ||
			s.Path == "cmd/main.go" || s.Path == "internal/database/store_test.go" ||
			s.Path == "vendor/github.com/external/lib.go" {
			fmt.Printf("%-40s | %.4f | %.4f | %.4f | %.4f | %.4f | %7d\n",
				truncate(s.Path, 40),
				s.Score,
				s.RecencyScore,
				s.FrequencyScore,
				s.ImportanceScore,
				s.RelationshipScore,
				s.AccessCount)
		}
	}
}

func generateContent(approxTokens int) string {
	// Rough approximation: 1 token ~= 4 characters
	chars := approxTokens * 4
	content := "package example\n\n// This is generated content for demo purposes\n"

	for len(content) < chars {
		content += "func DoSomething() { /* implementation */ }\n"
	}

	return content
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "..." + s[len(s)-maxLen+3:]
}
