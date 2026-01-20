package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/app"
	"golang.org/x/time/rate"
)

type AgentServer struct {
	Port         string
	App          *app.AgentContext
	approvalMap  map[string]chan bool
	approvalLock sync.Mutex
}

func New(port string, appCtx *app.AgentContext) *AgentServer {
	srv := &AgentServer{
		Port:        port,
		App:         appCtx,
		approvalMap: make(map[string]chan bool),
	}

	appCtx.Engine.Enforcer.SetInteractionHandler(srv.WaitForApproval)
	return srv
}

func (s *AgentServer) Start() error {
	mux := http.NewServeMux()
	limits := s.App.Limits
	cfg := s.App.Config

	var allowedOrigins []string
	if cfg != nil {
		allowedOrigins = cfg.Server.AllowedOrigins
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	rLimit := rate.Limit(limits.RateLimitPerMinute / 60.0)
	rateLimiter := NewRateLimiter(rLimit, limits.RateLimitBurst)

	apiStack := []Middleware{
		RecoveryMiddleware(s.App.Logger),
		BodyLimitMiddleware(limits.MaxBodySize),
		RequestID(),
		RequestLogger(s.App.Logger),
		rateLimiter.Middleware(),
		EnableCORS(allowedOrigins),
	}

	mux.HandleFunc("/agent/task", s.Chain(s.handleAgentTask, apiStack...))
	mux.HandleFunc("/agent/approve", s.Chain(s.handleApprovalResponse, apiStack...))

	mux.HandleFunc("/agent/context", s.Chain(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetContext(w, r)
		case http.MethodPost:
			s.handleAddContext(w, r)
		case http.MethodDelete:
			s.handleDeleteContext(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}, apiStack...))

	mux.HandleFunc("/health", s.Chain(s.handleHealth, EnableCORS(allowedOrigins)))
	mux.HandleFunc("/metrics", s.Chain(s.handleMetrics, EnableCORS(allowedOrigins)))

	server := &http.Server{
		Addr:    ":" + s.Port,
		Handler: mux,
	}

	go func() {
		s.App.Logger.Info(fmt.Sprintf("🚀 Agent Daemon listening on :%s", s.Port))

		s.App.Logger.Info(fmt.Sprintf("🛡️  Policy: %s (Mode: %s)",
			s.App.Engine.Enforcer.Policy.Name,
			s.App.Engine.Enforcer.Policy.Mode))

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.App.Logger.Error(fmt.Sprintf("Server failed: %v", err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	s.App.Logger.Info(fmt.Sprintf("🛑 Received signal: %v. Shutting down...", sig))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		s.App.Logger.Error(fmt.Sprintf("Server forced to shutdown: %v", err))
		return err
	}

	s.App.Logger.Info("✅ Server exited gracefully")
	return nil
}

// WaitForApproval matches the new agent.InteractionFunc signature
func (s *AgentServer) WaitForApproval(req agent.ApprovalRequest) bool {
	reqID := fmt.Sprintf("%d", time.Now().UnixNano())
	responseChan := make(chan bool)

	s.approvalLock.Lock()
	s.approvalMap[reqID] = responseChan
	s.approvalLock.Unlock()

	defer func() {
		s.approvalLock.Lock()
		delete(s.approvalMap, reqID)
		s.approvalLock.Unlock()
	}()

	// We log the request here since we can't easily push to a stream
	// unless we are inside the handler loop (which is handled by the closure in handlers.go).
	// This fallback is mostly for when the server is running without an active SSE stream
	// or in a background context.
	s.App.Logger.Info(fmt.Sprintf("⏳ Waiting for approval: %s %s", req.Tool, req.Args))

	select {
	case approved := <-responseChan:
		return approved
	case <-time.After(60 * time.Second):
		s.App.Logger.Warn(fmt.Sprintf("Approval timed out for %s", reqID))
		return false
	}
}

func (s *AgentServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"mode":   string(s.App.Engine.Enforcer.Policy.Mode),
	})
}
