package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainAgent "github.com/david22573/codepicker/domain/agent"
)

// RemoteToolPayload defines the RPC structure for distributed execution.
type RemoteToolPayload struct {
	ToolName string `json:"tool_name"`
	Args     string `json:"args"`
}

// RemoteToolResult defines the worker node's response.
type RemoteToolResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// RemoteTool proxy implementation offloads heavy processing to external worker nodes.
type RemoteTool struct {
	name        string
	description string
	workerURL   string
	client      *http.Client
}

// NewRemoteTool initializes a proxy that complies with the standard agent.Tool interface.
func NewRemoteTool(name, description, workerURL string) *RemoteTool {
	return &RemoteTool{
		name:        name,
		description: description,
		workerURL:   workerURL,
		client:      &http.Client{Timeout: 10 * time.Minute}, // Extended timeout for heavy remote workloads
	}
}

func (r *RemoteTool) Name() string {
	return r.name
}

func (r *RemoteTool) Description() string {
	return r.description + " [Executes Remotely]"
}

func (r *RemoteTool) Execute(ctx context.Context, args string) (string, error) {
	payload := RemoteToolPayload{
		ToolName: r.name,
		Args:     args,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("remote tool marshal failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.workerURL, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote execution network failure: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read remote response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("remote node returned status %d: %s", resp.StatusCode, string(body))
	}

	var result RemoteToolResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode remote result: %w", err)
	}

	if result.Error != "" {
		return result.Output, fmt.Errorf("remote execution error: %s", result.Error)
	}

	return result.Output, nil
}

// MapDistributed overrides specific local tools with remote proxies based on configuration.
func MapDistributed(localTools []domainAgent.Tool, workerNodeURL string, distributedTargets []string) []domainAgent.Tool {
	targetMap := make(map[string]bool)
	for _, t := range distributedTargets {
		targetMap[t] = true
	}

	var mapped []domainAgent.Tool
	for _, tool := range localTools {
		if targetMap[tool.Name()] {
			// Replace local tool with the Remote proxy version
			mapped = append(mapped, NewRemoteTool(tool.Name(), tool.Description(), workerNodeURL))
		} else {
			mapped = append(mapped, tool)
		}
	}
	return mapped
}