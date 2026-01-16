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
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/logger"
	"golang.org/x/time/rate"
)

type AgentServer struct {
	Port         string
	Engine       *agent.Engine
	Logger       logger.Logger
	approvalMap  map[string]chan bool
	approvalLock sync.Mutex
}

func New(port string, eng *agent.Engine, log logger.Logger) *AgentServer {
	srv := &AgentServer{
		Port:        port,
		Engine:      eng,
		Logger:      log,
		approvalMap: make(map[string]chan bool),
	}
	eng.ApprovalCallback = srv.WaitForApproval
	return srv
}

func (s *AgentServer) Start() error {
	mux := http.NewServeMux()
	limits := config.DefaultLimits()

	// Load config for security settings
	cfg, _ := config.LoadConfigFile("")
	var allowedOrigins []string
	if cfg != nil {
		allowedOrigins = cfg.Server.AllowedOrigins
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	rLimit := rate.Limit(limits.RateLimitPerMinute / 60.0)
	rateLimiter := NewRateLimiter(rLimit, limits.RateLimitBurst)

	// Middleware stack for API endpoints
	apiStack := []Middleware{
		RecoveryMiddleware(s.Logger),
		BodyLimitMiddleware(limits.MaxBodySize),
		RequestID(),
		RequestLogger(s.Logger),
		rateLimiter.Middleware(),
		EnableCORS(allowedOrigins),
	}

	// Endpoints
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

	// Observability Endpoints (No rate limit, minimal middleware)
	mux.HandleFunc("/health", s.Chain(s.handleHealth, EnableCORS(allowedOrigins)))
	mux.HandleFunc("/metrics", s.Chain(s.handleMetrics, EnableCORS(allowedOrigins)))

	server := &http.Server{
		Addr:    ":" + s.Port,
		Handler: mux,
	}

	// Server run loop
	go func() {
		s.Logger.Info(fmt.Sprintf("🚀 Agent Daemon listening on :%s", s.Port))
		s.Logger.Info(fmt.Sprintf("🛡️  Rate Limit: %.2f req/min (Burst: %d)", limits.RateLimitPerMinute, limits.RateLimitBurst))
		s.Logger.Info("📊 Metrics available at /metrics")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Logger.Error(fmt.Sprintf("Server failed: %v", err))
			os.Exit(1)
		}
	}()

	// Graceful Shutdown Logic
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal
	sig := <-quit
	s.Logger.Info(fmt.Sprintf("🛑 Received signal: %v. Shutting down...", sig))

	// Create context with timeout for cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		s.Logger.Error(fmt.Sprintf("Server forced to shutdown: %v", err))
		return err
	}

	s.Logger.Info("✅ Server exited gracefully")
	return nil
}

func (s *AgentServer) WaitForApproval(cmdStr, reason string) bool {
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

	select {
	case approved := <-responseChan:
		return approved
	case <-time.After(60 * time.Second):
		s.Logger.Warn(fmt.Sprintf("Approval timed out for %s", reqID))
		return false
	}
}

func (s *AgentServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
