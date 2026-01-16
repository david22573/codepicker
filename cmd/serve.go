package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	cpErrors "github.com/david22573/codepicker/internal/errors"
	"github.com/david22573/codepicker/internal/paths"
	"github.com/david22573/codepicker/internal/scanner"
	"github.com/david22573/codepicker/internal/server"
	"github.com/david22573/codepicker/internal/writer"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/spf13/cobra"
)

var port string

type ServerState struct {
	Port string
}

var serverState ServerState

const (
	MaxQueryLength = 25000       // characters
	MaxModelLength = 100         // characters
	MaxBodySize    = 1024 * 1024 // 1MB Max request body
)

var safeModelName = regexp.MustCompile(`^[a-zA-Z0-9\-\_\.\:\/]+$`)

type AskRequest struct {
	Query     string `json:"query"`
	Model     string `json:"model"`
	Focus     string `json:"focus"`
	Overwrite bool   `json:"overwrite"`
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker agent daemon",
	RunE: func(cmd *cobra.Command, args []string) error {

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY environment variable required")
		}

		absSrc, err := filepath.Abs(srcDir)
		if err != nil {
			return fmt.Errorf("failed to resolve source dir: %w", err)
		}

		client := openrouter.NewClient(apiKey)

		engine, err := agent.NewEngine(
			client,
			"xiaomi/mimo-v2-flash:free",
			absSrc,
			appLogger,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize agent engine: %w", err)
		}

		srv := server.New(port, engine, appLogger)
		return srv.Start()
	},
}

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

	var req AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appLogger.Error(fmt.Sprintf("Invalid JSON or body too large: %v", err))
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}
	if len(req.Query) > MaxQueryLength {
		http.Error(w, "Query too long", http.StatusBadRequest)
		return
	}

	model := req.Model
	if model == "" {
		model = "xiaomi/mimo-v2-flash:free"
	}
	if len(model) > MaxModelLength || !safeModelName.MatchString(model) {
		http.Error(w, "Invalid model name", http.StatusBadRequest)
		return
	}

	// SECURITY FIX (Phase 0): Sanitize and capture the clean path
	var cleanFocus string
	if req.Focus != "" {
		var err error
		cleanFocus, err = paths.Sanitize(req.Focus)
		if err != nil {
			appLogger.Warn(fmt.Sprintf("Blocked unsafe focus path: %s", req.Focus))
			http.Error(w, "Invalid focus path", http.StatusBadRequest)
			return
		}
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		http.Error(w, "Server configuration error: API key missing", http.StatusInternalServerError)
		return
	}

	absOut, err := paths.Sanitize(outPath)
	if err != nil {
		http.Error(w, "Server configuration error: Invalid output path", http.StatusInternalServerError)
		return
	}

	var contextBytes []byte
	_, statErr := os.Stat(absOut)
	cacheExists := statErr == nil

	if cacheExists && !req.Overwrite {
		appLogger.Info("⚡ Cache Hit: Using existing context file")
		contextBytes, err = os.ReadFile(absOut)
		if err != nil {
			http.Error(w, "Failed to read cache file", http.StatusInternalServerError)
			return
		}
	} else {
		status := "🔄 Cache Miss"
		if req.Overwrite {
			status = "🔨 Force Overwrite"
		}
		appLogger.Info(fmt.Sprintf("%s: Regenerating context...", status))

		wStrat := writer.NewConcatStrategy(absOut, minify, false)
		if err := wStrat.Init(); err != nil {
			http.Error(w, fmt.Sprintf("Writer init failed: %v", err), http.StatusInternalServerError)
			return
		}

		absSrc, err := paths.Sanitize(srcDir)
		if err != nil {
			http.Error(w, "Invalid source directory", http.StatusInternalServerError)
			return
		}

		cfg := config.NewConfig()
		s := scanner.NewScanner(absSrc, wStrat, cfg, appLogger)

		if err := s.Scan(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Scan failed: %v", err), http.StatusInternalServerError)
			return
		}
		wStrat.Close()

		contextBytes, err = os.ReadFile(absOut)
		if err != nil {
			http.Error(w, "Failed to read generated context", http.StatusInternalServerError)
			return
		}
	}

	contextType := "Codebase"
	// SECURITY FIX (Phase 0): Use cleanFocus
	if cleanFocus != "" {
		contextType = "Active File"
	}

	// Phase 1: Robust Context Gen Error Reporting
	if len(contextBytes) == 0 {
		agentErr := cpErrors.NewContextGenerationError(errors.New("empty context generated"))
		http.Error(w, agentErr.Error(), http.StatusInternalServerError)
		return
	}

	systemMsg := fmt.Sprintf(
		"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.\n"+
			"CRITICAL: Return code inside Markdown code blocks (```language ... ```). Do not output raw text without blocks.",
		time.Now().Format("2006-01-02"), contextType,
	)

	userMsg := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", string(contextBytes), req.Query)

	client := openrouter.NewClient(apiKey)
	chatReq := openrouter.ChatCompletionRequest{
		Model: model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Stream: true,
	}

	stream, err := client.CreateChatCompletionStream(r.Context(), chatReq)
	if err != nil {
		appLogger.Error(fmt.Sprintf("OpenRouter Error: %v", err))
		http.Error(w, "Upstream API Error", http.StatusBadGateway)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	for {
		select {
		case <-r.Context().Done():
			appLogger.Info("Client disconnected, aborting stream.")
			return
		default:
		}

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
	serveCmd.Flags().StringVarP(&port, "port", "p", "22573", "Port to listen on")
}
