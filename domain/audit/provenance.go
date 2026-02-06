package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Provenance holds the forensic data about an automated change.
type Provenance struct {
	Model        string
	Task         string
	ContextID    string
	ContextHash  string
	AnalysisHash string
	PolicyHash   string
}

// FormatCommitMessage generates the standardized forensic commit message.
func (p *Provenance) FormatCommitMessage() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("fix: %s (auto-generated)\n\n", p.Task))

	sb.WriteString("🤖 CodePicker Verification: PASSED\n")
	sb.WriteString("This change was analyzed, patched, and verified automatically.\n\n")

	sb.WriteString(fmt.Sprintf("Model: %s\n", p.Model))
	sb.WriteString(fmt.Sprintf("Context-ID: %s\n", p.ContextID))
	sb.WriteString(fmt.Sprintf("Context-Hash: %s\n", p.ContextHash))
	sb.WriteString(fmt.Sprintf("Analysis-Hash: %s\n", p.AnalysisHash))

	if p.PolicyHash != "" {
		sb.WriteString(fmt.Sprintf("Policy-Hash: %s\n", p.PolicyHash))
	}

	return sb.String()
}

// CalculateHash is a helper to generate SHA256 hashes for strings to ensure integrity.
func CalculateHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
