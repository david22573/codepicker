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

	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/event"
	"github.com/spf13/cobra"
)

var (
	servePort     int
	daemonAPIKey  string
	daemonCwd     string
	daemonVerbose bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the CodePicker background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		daemonAPIKey = getAPIKeyOrExit("serve")
		daemonCwd, _ = os.Getwd()
		daemonVerbose = GetVerbose()

		// Enable async channel-based approvals for the daemon
		policy.DaemonMode = true

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

	// 3. Async Approval Endpoint
	mux.HandleFunc("/agent/approve", handleApproval)

	// 4. Context UI stub
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

func handleApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TaskID string `json:"task_id"`
		Action string `json:"action"`
		Blocks string `json:"blocks"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	session := policy.GetSession(req.TaskID)
	if session == nil {
		http.Error(w, "Invalid or expired task_id", http.StatusBadRequest)
		return
	}

	// Unblock the waiting tool execution using select to avoid deadlocks
	select {
	case session.RespCh <- policy.ApprovalResponse{
		Action: req.Action,
		Blocks: req.Blocks,
	}:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "No pending approval request for this task", http.StatusConflict)
	}
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

	// Generate a unique task ID and attach to context
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(r.Context())
	ctx = context.WithValue(ctx, policy.TaskIDKey, taskID)
	defer cancel()

	// Register session channels for this request
	session := policy.GetOrCreateSession(taskID)
	defer policy.CleanupSession(taskID)

	// Create a per-request container to ensure thread safety
	container, err := app.NewContainer(daemonAPIKey, daemonCwd, "", false, false, daemonVerbose)
	if err != nil {
		http.Error(w, "Failed to initialize per-request container: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer container.Close()

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

	eventCh := container.EventBus.Subscribe()

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)

		primer := container.ProjectPrimer.GenerateShallow()
		plan, err := container.Planner.CreatePlan(ctx, query, "", primer)
		if err != nil {
			sendSSE(w, flusher, map[string]any{"type": "error", "msg": "Planning failed: " + err.Error()})
			return
		}

		container.PlanExecutor.SetAutoConfirm(true)

		err = container.PlanExecutor.Execute(ctx, plan)
		if err != nil {
			sendSSE(w, flusher, map[string]any{"type": "error", "msg": "Execution failed: " + err.Error()})
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-doneCh:
			sendSSE(w, flusher, map[string]any{"type": "done"})
			return

		// Phase 5: Intercept Approval Requests and push to Neovim with the task_id
		case req := <-session.ReqCh:
			sendSSE(w, flusher, map[string]any{
				"type":     "approval_required",
				"task_id":  taskID,
				"filename": req.Filename,
				"blocks":   req.Blocks,
			})

		case ev, open := <-eventCh:
			if !open {
				return
			}

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
