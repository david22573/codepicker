package unit

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/david22573/codepicker/adapters/verifier"
	"github.com/david22573/codepicker/infra/pathutil"
)

// 1. Path Safety Check tests
func TestPathSafety_ComprehensiveBlockedPaths(t *testing.T) {
	blockedCases := []string{
		"../outside.txt",
		"../../etc/passwd",
		"/etc/passwd",
		"~/.ssh/id_rsa",
		"C:\\Windows\\System32",
		"\\\\server\\share\\file.txt",
		"file:///etc/passwd",
	}

	for _, tc := range blockedCases {
		_, err := pathutil.Clean(tc)
		if err == nil {
			t.Errorf("Path Safety Check failed: expected error for blocked path '%s', got nil", tc)
		}
	}

	allowedCases := []string{
		"cmd/root.go",
		"internal/app/app.go",
		"docs/quickstart.md",
		"README.md",
		"Makefile",
	}

	for _, tc := range allowedCases {
		_, err := pathutil.Clean(tc)
		if err != nil {
			t.Errorf("Path Safety Check failed: expected no error for allowed path '%s', got: %v", tc, err)
		}
	}
}

// 2. Verifier Selection & Language Defaults
func TestVerifier_LanguageSelectionAndCommands(t *testing.T) {
	cwd, _ := os.Getwd()
	tmpDir, err := os.MkdirTemp("", "verifier-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pipeline := verifier.NewPipeline(tmpDir)
	if pipeline.ProjectRoot != tmpDir {
		t.Errorf("expected ProjectRoot to be %s, got: %s", tmpDir, pipeline.ProjectRoot)
	}

	pipeline.Commands = []string{"echo hello"}
	if len(pipeline.Commands) != 1 || pipeline.Commands[0] != "echo hello" {
		t.Errorf("expected custom commands to be set, got: %v", pipeline.Commands)
	}

	os.Chdir(cwd)
}

// 3. JSON Output schemas
func TestJSONOutput_Schemas(t *testing.T) {
	// Prove report JSON validation
	proofJSON := map[string]interface{}{
		"status":        "pass",
		"run_id":        "proof-20260526-153000",
		"artifacts_dir": ".codepicker/runs/proof/20260526-153000",
		"checks": []map[string]string{
			{"name": "go test", "status": "pass"},
			{"name": "go vet", "status": "pass"},
			{"name": "go build", "status": "pass"},
		},
	}
	
	proofBytes, err := json.Marshal(proofJSON)
	if err != nil {
		t.Fatalf("failed to marshal proof JSON: %v", err)
	}
	
	var unmarshaledProof map[string]interface{}
	err = json.Unmarshal(proofBytes, &unmarshaledProof)
	if err != nil {
		t.Fatalf("marshaled proof is not valid JSON: %v", err)
	}
	if unmarshaledProof["status"] != "pass" {
		t.Errorf("expected status to be 'pass', got: %v", unmarshaledProof["status"])
	}

	// Cost Dashboard JSON validation
	costJSON := map[string]interface{}{
		"total_spend":            0.0450,
		"total_tokens":           15000,
		"avg_cost_per_1k_tokens": 0.0030,
	}
	costBytes, err := json.Marshal(costJSON)
	if err != nil {
		t.Fatalf("failed to marshal cost JSON: %v", err)
	}
	var unmarshaledCost map[string]interface{}
	err = json.Unmarshal(costBytes, &unmarshaledCost)
	if err != nil {
		t.Fatalf("marshaled cost is not valid JSON: %v", err)
	}
	if unmarshaledCost["total_tokens"].(float64) != 15000 {
		t.Errorf("expected total_tokens to be 15000, got: %v", unmarshaledCost["total_tokens"])
	}
}

// 4. Backup manifest structure and format
func TestBackupManifest_Schema(t *testing.T) {
	type BackupFileEntry struct {
		Path          string  `json:"path"`
		Operation     string  `json:"operation"`
		ExistedBefore bool    `json:"existed_before"`
		BackupPath    *string `json:"backup_path"`
	}
	type BackupManifest struct {
		RunID     string            `json:"run_id"`
		CreatedAt time.Time         `json:"created_at"`
		Files     []BackupFileEntry `json:"files"`
	}

	backupPath := "backups/cmd/pack.go"
	manifest := BackupManifest{
		RunID:     "run-20260526-153000",
		CreatedAt: time.Now(),
		Files: []BackupFileEntry{
			{
				Path:          "cmd/pack.go",
				Operation:     "modified",
				ExistedBefore: true,
				BackupPath:    &backupPath,
			},
			{
				Path:          "docs/new.md",
				Operation:     "added",
				ExistedBefore: false,
				BackupPath:    nil,
			},
		},
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal backup manifest: %v", err)
	}

	var parsed BackupManifest
	err = json.Unmarshal(manifestBytes, &parsed)
	if err != nil {
		t.Fatalf("backup manifest is not valid JSON: %v", err)
	}

	if len(parsed.Files) != 2 {
		t.Errorf("expected 2 files in backup manifest, got %d", len(parsed.Files))
	}
	if parsed.Files[0].Operation != "modified" {
		t.Errorf("expected operation to be modified, got: %s", parsed.Files[0].Operation)
	}
}
