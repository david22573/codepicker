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

	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the CodePicker background daemon",
	Long:  `Starts a long-running HTTP server to handle requests from the Neovim plugin.`,
	Run: func(cmd *cobra.Command, args []string) {
		startServer(servePort)
	},
}

func init() {
	// Assumes your root command is called rootCmd. Adjust if yours is named differently.
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

	// 3. Context & Approval Stubs
	mux.HandleFunc("/agent/approve", handleAgentApprove)
	mux.HandleFunc("/agent/context", handleAgentContext)

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Run the server in a goroutine so it doesn't block
	go func() {
		log.Printf("🚀 Agent Daemon listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server fucked up and crashed: %v", err)
		}
	}()

	// Graceful Shutdown Channel
	quit := make(chan os.Signal, 1)
	// Catch Ctrl+C (SIGINT) and kill commands (SIGTERM)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit // Block here until a signal is received
	log.Println("\n🛑 Shutting down server gracefully...")

	// Give active connections 5 seconds to finish their shit before forcing exit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly.")
}

// handleAgentTask streams JSON events back to Neovim using SSE
func handleAgentTask(w http.ResponseWriter, r *http.Request) {
	// Require GET method
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	// Essential headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// --- WIRING STUB: Replace this with your actual agent executor ---

	sendSSE(w, flusher, map[string]interface{}{
		"type":    "thought",
		"content": fmt.Sprintf("Received task: `%s`\nSpinning up LLM context...\n", query),
	})

	// Simulate work so you can see the UI stream
	time.Sleep(1 * time.Second)

	// Tell Neovim the task is done
	sendSSE(w, flusher, map[string]interface{}{
		"type": "done",
	})
}

// Helper to format and flush SSE data
func sendSSE(w http.ResponseWriter, flusher http.Flusher, payload interface{}) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", string(bytes))
	flusher.Flush()
}

func handleAgentApprove(w http.ResponseWriter, r *http.Request) {
	// Needs to handle POST requests from the Neovim Sentinel UI
	w.WriteHeader(http.StatusOK)
}

func handleAgentContext(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		// Return empty context array to stop Neovim from shitting itself
		w.Write([]byte(`{"files": []}`))
	} else {
		w.WriteHeader(http.StatusOK)
	}
}
