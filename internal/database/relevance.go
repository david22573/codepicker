package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// RelevanceScore represents the calculated relevance score for a memory file
type RelevanceScore struct {
	Path              string
	Content           string
	TokenCount        int
	Score             float64
	RecencyScore      float64
	FrequencyScore    float64
	ImportanceScore   float64
	RelationshipScore float64
	LastAccessed      time.Time
	AccessCount       int
}

// RelevanceScorer calculates intelligent relevance scores for memory files
type RelevanceScorer struct {
	db *sql.DB
}

// NewRelevanceScorer creates a new relevance scorer
func NewRelevanceScorer(db *sql.DB) *RelevanceScorer {
	return &RelevanceScorer{db: db}
}

// CalculateRelevanceScores computes relevance scores for all files in working memory
func (rs *RelevanceScorer) CalculateRelevanceScores() ([]RelevanceScore, error) {
	rows, err := rs.db.Query(`
		SELECT path, content, token_count, last_accessed, access_count 
		FROM memory_files 
		ORDER BY last_accessed DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []RelevanceScore
	now := time.Now()

	for rows.Next() {
		var rs RelevanceScore
		if err := rows.Scan(&rs.Path, &rs.Content, &rs.TokenCount, &rs.LastAccessed, &rs.AccessCount); err != nil {
			continue
		}

		// Calculate component scores
		rs.RecencyScore = calculateRecencyScore(rs.LastAccessed, now)
		rs.FrequencyScore = calculateFrequencyScore(rs.AccessCount)
		rs.ImportanceScore = calculateImportanceScore(rs.Path, rs.Content)
		rs.RelationshipScore = 0.0 // Will be calculated in second pass

		scores = append(scores, rs)
	}

	// Second pass: calculate relationship scores
	for i := range scores {
		scores[i].RelationshipScore = calculateRelationshipScore(&scores[i], scores)
	}

	// Calculate final weighted scores
	for i := range scores {
		scores[i].Score = calculateFinalScore(&scores[i])
	}

	return scores, nil
}

// calculateRecencyScore: Files accessed recently get higher scores
// Uses exponential decay: score drops by 50% every 10 minutes
func calculateRecencyScore(lastAccessed, now time.Time) float64 {
	minutesSinceAccess := now.Sub(lastAccessed).Minutes()

	// Exponential decay with half-life of 10 minutes
	halfLife := 10.0
	score := 1.0 / (1.0 + (minutesSinceAccess / halfLife))

	// Boost for very recent access (< 1 minute)
	if minutesSinceAccess < 1.0 {
		score = 1.0
	}

	return score
}

// calculateFrequencyScore: Files accessed frequently are more important
func calculateFrequencyScore(accessCount int) float64 {
	// Logarithmic scaling to prevent domination by old frequently-accessed files
	if accessCount <= 1 {
		return 0.1
	}

	// Score increases with access count but with diminishing returns
	// Max score ~0.9 at 100+ accesses
	score := 0.1 + (0.8 * (1.0 - 1.0/(1.0+float64(accessCount)/10.0)))

	return score
}

// calculateImportanceScore: Certain files are inherently more important
func calculateImportanceScore(path, content string) float64 {
	score := 0.5 // Base score

	fileName := filepath.Base(path)
	fileExt := filepath.Ext(path)

	// Configuration files are critical
	criticalFiles := map[string]float64{
		"go.mod":             1.0,
		"go.sum":             0.9,
		"package.json":       1.0,
		"Cargo.toml":         1.0,
		"Dockerfile":         0.8,
		"docker-compose.yml": 0.8,
		"Makefile":           0.8,
		".env":               0.7,
		"README.md":          0.6,
	}

	if boost, exists := criticalFiles[fileName]; exists {
		score = boost
		return score
	}

	// Core infrastructure files
	if strings.Contains(path, "main.") || fileName == "main.go" {
		score += 0.3
	}

	// Type definitions and interfaces are important
	if strings.Contains(fileName, "types") || strings.Contains(fileName, "interface") {
		score += 0.2
	}

	// Schema and migration files
	if strings.Contains(fileName, "schema") || strings.Contains(fileName, "migration") {
		score += 0.25
	}

	// Configuration files
	if strings.Contains(fileName, "config") {
		score += 0.2
	}

	// Test files are less critical (can be re-read if needed)
	if strings.HasSuffix(fileName, "_test"+fileExt) {
		score -= 0.2
	}

	// Generated files are less important
	if strings.Contains(content, "Code generated") ||
		strings.Contains(content, "auto-generated") ||
		strings.Contains(path, "generated") {
		score -= 0.3
	}

	// Vendor and node_modules are low priority
	if strings.Contains(path, "vendor/") || strings.Contains(path, "node_modules/") {
		score -= 0.4
	}

	// Core packages get boost
	corePackages := []string{"internal/agent", "internal/database", "pkg/"}
	for _, pkg := range corePackages {
		if strings.Contains(path, pkg) {
			score += 0.15
			break
		}
	}

	// Deep nesting reduces importance (likely utility files)
	pathDepth := strings.Count(path, string(filepath.Separator))
	if pathDepth > 4 {
		score -= 0.1 * float64(pathDepth-4)
	}

	// Clamp between 0 and 1
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// calculateRelationshipScore: Files that import or are imported by other loaded files
func calculateRelationshipScore(file *RelevanceScore, allFiles []RelevanceScore) float64 {
	relationships := 0

	fileName := strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
	packagePath := strings.TrimSuffix(file.Path, filepath.Base(file.Path))

	// Check if this file is referenced by others
	for _, other := range allFiles {
		if other.Path == file.Path {
			continue
		}

		// Check for imports
		if strings.Contains(other.Content, file.Path) {
			relationships++
		}

		// Check for package imports
		if packagePath != "" && strings.Contains(other.Content, packagePath) {
			relationships++
		}

		// Check for type/function references
		if strings.Contains(other.Content, fileName) {
			relationships++
		}
	}

	// Check if this file references others
	for _, other := range allFiles {
		if other.Path == file.Path {
			continue
		}

		if strings.Contains(file.Content, other.Path) {
			relationships++
		}
	}

	// Logarithmic scaling
	score := 0.0
	if relationships > 0 {
		score = 0.8 * (1.0 - 1.0/(1.0+float64(relationships)/5.0))
	}

	return score
}

// calculateFinalScore: Weighted combination of all component scores
func calculateFinalScore(rs *RelevanceScore) float64 {
	// Weights for each component (must sum to 1.0)
	weights := struct {
		recency      float64
		frequency    float64
		importance   float64
		relationship float64
	}{
		recency:      0.35, // Recent access is most important
		frequency:    0.20, // How often accessed
		importance:   0.30, // Inherent file importance
		relationship: 0.15, // Connections to other files
	}

	score := (weights.recency * rs.RecencyScore) +
		(weights.frequency * rs.FrequencyScore) +
		(weights.importance * rs.ImportanceScore) +
		(weights.relationship * rs.RelationshipScore)

	return score
}

// SelectFilesForEviction returns files that should be evicted to meet token budget
func (rs *RelevanceScorer) SelectFilesForEviction(targetTokens int) ([]string, error) {
	scores, err := rs.CalculateRelevanceScores()
	if err != nil {
		return nil, err
	}

	if len(scores) == 0 {
		return nil, nil
	}

	// Calculate total tokens
	totalTokens := 0
	for _, s := range scores {
		totalTokens += s.TokenCount
	}

	if totalTokens <= targetTokens {
		return nil, nil // No eviction needed
	}

	// Sort by score (lowest first - candidates for eviction)
	sortByScore(scores)

	var toEvict []string
	tokensToRemove := totalTokens - targetTokens
	tokensRemoved := 0

	for i := 0; i < len(scores) && tokensRemoved < tokensToRemove; i++ {
		toEvict = append(toEvict, scores[i].Path)
		tokensRemoved += scores[i].TokenCount
	}

	return toEvict, nil
}

// sortByScore sorts RelevanceScore slice by score ascending (lowest first)
func sortByScore(scores []RelevanceScore) {
	// Simple bubble sort (fine for small arrays)
	n := len(scores)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if scores[j].Score > scores[j+1].Score {
				scores[j], scores[j+1] = scores[j+1], scores[j]
			}
		}
	}
}

// GetTopFiles returns the N most relevant files
func (rs *RelevanceScorer) GetTopFiles(n int) ([]RelevanceScore, error) {
	scores, err := rs.CalculateRelevanceScores()
	if err != nil {
		return nil, err
	}

	// Sort by score descending (highest first)
	sortByScoreDesc(scores)

	if len(scores) <= n {
		return scores, nil
	}

	return scores[:n], nil
}

// sortByScoreDesc sorts RelevanceScore slice by score descending (highest first)
func sortByScoreDesc(scores []RelevanceScore) {
	n := len(scores)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if scores[j].Score < scores[j+1].Score {
				scores[j], scores[j+1] = scores[j+1], scores[j]
			}
		}
	}
}

// DebugScores returns a formatted string showing relevance scores for all files
func (rs *RelevanceScorer) DebugScores() (string, error) {
	scores, err := rs.CalculateRelevanceScores()
	if err != nil {
		return "", err
	}

	sortByScoreDesc(scores)

	var sb strings.Builder
	sb.WriteString("=== RELEVANCE SCORES ===\n")
	sb.WriteString(fmt.Sprintf("%-50s | %5s | %5s | %5s | %5s | %5s | %6s\n",
		"File", "Final", "Recen", "Freq", "Imprt", "Relat", "Tokens"))
	sb.WriteString(strings.Repeat("-", 100) + "\n")

	for _, s := range scores {
		sb.WriteString(fmt.Sprintf("%-50s | %.3f | %.3f | %.3f | %.3f | %.3f | %6d\n",
			truncatePath(s.Path, 50),
			s.Score,
			s.RecencyScore,
			s.FrequencyScore,
			s.ImportanceScore,
			s.RelationshipScore,
			s.TokenCount))
	}

	return sb.String(), nil
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
