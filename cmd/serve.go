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

type ServerState struct {
	Port string
}

var serverState ServerState

type AskRequest struct {
	Query     string `json:"query"`
	Model     string `json:"model"`
	Focus     string `json:"focus"`
	Overwrite bool   `json:"overwrite"`
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codepicker daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if outPath == "" {
			absSrc, _ := filepath.Abs(srcDir)
			dirName := filepath.Base(absSrc)
			outPath = fmt.Sprintf("%s_context.md", dirName)
		}

		appLogger.Info(fmt.Sprintf("🚀 Daemon started on port %s", serverState.Port))
		appLogger.Info(fmt.Sprintf("📂 Source: %s | 💾 Cache: %s", srcDir, outPath))

		http.HandleFunc("/ask", enableCORS(handleAsk))

		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})

		return http.ListenAndServe(":"+serverState.Port, nil)
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

	var req AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		appLogger.Error(fmt.Sprintf("Invalid JSON: %v", err))
		http.Error(w, fmt.Sprintf("Invalid JSON body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		http.Error(w, "OPENROUTER_API_KEY not set on server", http.StatusUnauthorized)
		return
	}

	absOut, err := paths.Sanitize(outPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid output path: %v", err), http.StatusInternalServerError)
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

		absSrc, _ := paths.Sanitize(srcDir)
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
	if req.Focus != "" {
		contextType = "Active File"
	}

	systemMsg := fmt.Sprintf(
		"You are an expert coding assistant. Date: %s. Use the provided %s Context to answer.\n"+
			"CRITICAL: Return code inside Markdown code blocks (```language ... ```). Do not output raw text without blocks.",
		time.Now().Format("2006-01-02"), contextType,
	)

	userMsg := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", string(contextBytes), req.Query)

	model := req.Model
	if model == "" {
		model = "xiaomi/mimo-v2-flash:free"
	}

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
		http.Error(w, fmt.Sprintf("OpenRouter Error: %v", err), http.StatusInternalServerError)
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
	serveCmd.Flags().StringVarP(&serverState.Port, "port", "p", "22573", "Port to listen on")
}

