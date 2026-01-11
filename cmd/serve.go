package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

// ServerState holds configuration ensuring we respect flags passed during startup
type ServerState struct {
	Port string
}

var serverState ServerState

// AskRequest defines the JSON payload from the Lua client
type AskRequest struct {
	Query     string `json:"query"`
	Model     string `json:"model"`
	Focus     string `json:"focus"`     // Optional: Focus on a specific file
	Overwrite bool   `json:"overwrite"` // True = Force regeneration (-y)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure output path defaults are set (logic borrowed from root.go)
		if outPath == "" {
			absSrc, _ := filepath.Abs(srcDir)
			dirName := filepath.Base(absSrc)
			outPath = fmt.Sprintf("%s_context.md", dirName)
		}

		appLogger.Info(fmt.Sprintf("🚀 Daemon started on port %s", serverState.Port))
		appLogger.Info(fmt.Sprintf("📂 Source: %s | 💾 Cache: %s", srcDir, outPath))

		http.HandleFunc("/ask", handleAsk)
		return http.ListenAndServe(":"+serverState.Port, nil)
	},
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Parse Request
	var req AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// 2. Validate API Key
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		http.Error(w, "OPENROUTER_API_KEY not set on server", http.StatusUnauthorized)
		return
	}

	// 3. Cache Logic
	absOut, err := paths.Sanitize(outPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid output path: %v", err), http.StatusInternalServerError)
		return
	}

	var contextBytes []byte
	_, statErr := os.Stat(absOut)
	cacheExists := statErr == nil

	// IF cache exists AND we are NOT overwriting -> USE CACHE
	if cacheExists && !req.Overwrite {
		appLogger.Info("⚡ Cache Hit: Using existing context file")
		contextBytes, err = os.ReadFile(absOut)
		if err != nil {
			http.Error(w, "Failed to read cache file", http.StatusInternalServerError)
			return
		}
	} else {
		// ELSE -> REGENERATE
		status := "🔄 Cache Miss"
		if req.Overwrite {
			status = "🔨 Force Overwrite"
		}
		appLogger.Info(fmt.Sprintf("%s: Regenerating context...", status))

		// Initialize Writer
		wStrat := writer.NewConcatStrategy(absOut, minify, false)
		if err := wStrat.Init(); err != nil {
			http.Error(w, fmt.Sprintf("Writer init failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Initialize Scanner
		absSrc, _ := paths.Sanitize(srcDir)
		cfg := config.NewConfig()
		if includeExts != "" {
			// Apply include flags if they were passed to 'serve'
			// Note: rudimentary parsing here, robust app might share config logic better
			// For now, relies on global 'includeExts' being set by Cobra
		}

		s := scanner.NewScanner(absSrc, wStrat, cfg, appLogger)
		if err := s.Scan(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Scan failed: %v", err), http.StatusInternalServerError)
			return
		}
		wStrat.Close()

		// Read the fresh file
		contextBytes, err = os.ReadFile(absOut)
		if err != nil {
			http.Error(w, "Failed to read generated context", http.StatusInternalServerError)
			return
		}
	}

	// 4. Construct Prompt
	contextType := "Codebase"
	if req.Focus != "" {
		contextType = "Active File"
		// If focusing, we might want to strictly limit context, 
		// but for now we append the full context + focus instruction.
	}

	systemMsg := fmt.Sprintf(
		"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.\n"+
			"CRITICAL: Return code inside Markdown code blocks (```language ... ```). Do not output raw text without blocks.",
		time.Now().Format("2006-01-02"), contextType,
	)

	userMsg := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", string(contextBytes), req.Query)

	// 5. Stream Response from OpenRouter
	client := openrouter.NewClient(apiKey)
	chatReq := openrouter.ChatCompletionRequest{
		Model: req.Model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Stream: true,
	}

	stream, err := client.CreateChatCompletionStream(r.Context(), chatReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("OpenRouter Error: %v", err), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Headers for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx-friendly

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(resp.Choices) > 0 {
			if resp.Choices[0].Delta != nil {
				content, ok := resp.Choices[0].Delta.Content.(string)
				if ok {
					fmt.Fprint(w, content)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&serverState.Port, "port", "p", "22573", "Port to listen on")
}
