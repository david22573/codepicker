package server

import (
	"context"
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

	// Register the server's approval method with the engine's enforcer
	appCtx.Engine.Enforcer.SetInteractionHandler(srv.WaitForApproval)
	return srv
}

func (s *AgentServer) Start() error {
	mux := http.NewServeMux()
	limits := s.App.Limits
	cfg := s.App.Config

	// 1. Configure CORS
	var allowedOrigins []string
	if cfg != nil {
		allowedOrigins = cfg.Server.AllowedOrigins
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	// 2. Configure Rate Limiter
	rLimit := rate.Limit(limits.RateLimitPerMinute / 60.0)
	rateLimiter := NewRateLimiter(rLimit, limits.RateLimitBurst)

	// 3. Security Checks
	authToken := os.Getenv("CODEPICKER_AUTH_TOKEN")
	if authToken == "" {
		s.App.Logger.Warn("\n⚠️  SECURITY WARNING ⚠️")
		s.App.Logger.Warn("Server starting WITHOUT authentication token (CODEPICKER_AUTH_TOKEN is empty).")
		s.App.Logger.Warn("The agent API is publicly accessible. Do not expose this port to the internet.\n")
	} else {
		s.App.Logger.Info("🔒 Auth Token detected. API is protected.")
	}

	// 4. Build Middleware Stack
	// Order matters: Inner <- Outer.
	// We want: CORS -> Auth -> RateLimit -> Logger -> ID -> Body -> Recovery -> Handler

	baseStack := []Middleware{
		RecoveryMiddleware(s.App.Logger),
		BodyLimitMiddleware(limits.MaxBodySize),
		RequestID(),
		RequestLogger(s.App.Logger),
		rateLimiter.Middleware(),
	}

	// Inject Auth if token exists
	if authToken != "" {
		baseStack = append(baseStack, AuthMiddleware(authToken))
	}

	// CORS is always outermost (to handle OPTIONS pre-flight without Auth)
	baseStack = append(baseStack, EnableCORS(allowedOrigins))

	// 5. Register Routes
	mux.HandleFunc("/agent/task", s.Chain(s.handleAgentTask, baseStack...))
	mux.HandleFunc("/agent/approve", s.Chain(s.handleApprovalResponse, baseStack...))

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
	}, baseStack...))

	// Health check is public (CORS only, no Auth/RateLimit usually needed, but keeping CORS)
	mux.HandleFunc("/health", s.Chain(s.handleHealth, EnableCORS(allowedOrigins)))

	server := &http.Server{
		Addr:    ":" + s.Port,
		Handler: mux,
	}

	// 6. Start Server in Goroutine
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

	// 7. Graceful Shutdown
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

// Chain applies middleware to a handler.
// Note: We iterate in reverse so the last middleware in slice becomes the outermost wrapper.
func (s *AgentServer) Chain(h http.HandlerFunc, middleware ...Middleware) http.HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

func (s *AgentServer) WaitForApproval(req agent.ApprovalRequest) agent.ApprovalResponse {
	reqID := fmt.Sprintf("%d", time.Now().UnixNano())

	// In a real scenario, you'd likely broadcast this ID via WebSocket or SSE to the frontend.
	// For now, we log it so the user can POST to /agent/approve with this ID.
	s.App.Logger.Warn(fmt.Sprintf("ACTION REQUIRED: Approval needed for %s. ID: %s", req.Tool, reqID))

	responseChan := make(chan bool)
	s.approvalLock.Lock()
	s.approvalMap[reqID] = responseChan
	s.approvalLock.Unlock()

	defer func() {
		s.approvalLock.Lock()
		delete(s.approvalMap, reqID)
		s.approvalLock.Unlock()
	}()

	select {
	case approved := <-responseChan:
		return agent.ApprovalResponse{Approved: approved, SessionScope: false}
	case <-time.After(60 * time.Second): // Auto-deny after 60s
		s.App.Logger.Warn(fmt.Sprintf("Approval timed out for %s", reqID))
		return agent.ApprovalResponse{Approved: false}
	}
}
