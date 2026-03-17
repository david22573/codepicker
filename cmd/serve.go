package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/event"
	"github.com/spf13/cobra"
)

var servePort int
var globalContainer *app.Container

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the CodePicker background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := getAPIKeyOrExit()
		cwd, _ := os.Getwd()

		// Initialize your full application container (DB, Tools, Agent)
		container, err := app.NewContainer(apiKey, cwd, "", false, false, GetVerbose())
		if err != nil {
			return fmt.Errorf("daemon init failed: %w", err)
		}

		globalContainer = container
		defer container.Close()

		startServer(servePort)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 22573, "Port to run the server on")
}

func startServer(port int) {
	mux := http.NewServeMux()

	// 1. Health Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 2. Core Agent Stream
	mux.HandleFunc("/agent/task", handleAgentTask)

	// 3. Stubs to prevent Neovim crashes
	mux.HandleFunc("/agent/approve", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/agent/context", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"files": []}`))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("🚀 Agent Daemon listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server crashed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("\n🛑 Shutting down daemon gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func handleAgentTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	sendSSE(w, flusher, map[string]any{
		"type":    "thought",
		"content": fmt.Sprintf("🚀 Task received: `%s`\nSpinning up LLM context...\n\n", query),
	})

	// Subscribe to your global Event Bus to catch Agent thoughts and tool executions
	eventCh := globalContainer.EventBus.Subscribe()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Run the planner and executor in the background
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)

		primer := globalContainer.ProjectPrimer.GenerateShallow()
		plan, err := globalContainer.Planner.CreatePlan(ctx, query, "", primer)
		if err != nil {
			sendSSE(w, flusher, map[string]any{"type": "error", "msg": "Planning failed: " + err.Error()})
			return
		}

		err = globalContainer.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			sendSSE(w, flusher, map[string]any{"type": "error", "msg": "Execution failed: " + err.Error()})
		}
	}()

	// Stream Events back to Neovim
	for {
		select {
		case <-ctx.Done():
			return // Neovim closed the connection

		case <-doneCh:
			// Agent finished all steps
			sendSSE(w, flusher, map[string]any{"type": "done"})
			return

		case ev, open := <-eventCh:
			if !open {
				return
			}

			// Map your internal Domain Events to Neovim SSE formats
			switch ev.Type {
			case event.EventAgentThought:
				if content, ok := ev.Payload["content"].(string); ok {
					sendSSE(w, flusher, map[string]any{"type": "thought", "content": content + "\n"})
				}
			case event.EventToolStart:
				if tool, ok := ev.Payload["tool"].(string); ok {
					sendSSE(w, flusher, map[string]any{"type": "thought", "content": fmt.Sprintf("\n🛠️ **Running Tool:** `%s`\n", tool)})
				}
			case event.EventPolicyBlock:
				if reason, ok := ev.Payload["reason"].(string); ok {
					sendSSE(w, flusher, map[string]any{"type": "error", "msg": "Policy Blocked: " + reason})
				}
			case event.EventError:
				if msg, ok := ev.Payload["error"].(string); ok {
					sendSSE(w, flusher, map[string]any{"type": "error", "msg": msg})
				}
			}
		}
	}
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, payload any) {
	bytes, err := json.Marshal(payload)
	if err == nil {
		fmt.Fprintf(w, "data: %s\n\n", string(bytes))
		flusher.Flush()
	}
}
