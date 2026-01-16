package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/david22573/codepicker/internal/agent"
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

	// SECURITY FIX: Rate limiter (10 requests per minute per IP, burst of 10)
	rateLimiter := NewRateLimiter(rate.Limit(10.0/60.0), 10)

	standardStack := []Middleware{
		RecoveryMiddleware(s.Logger),
		RequestID(),
		RequestLogger(s.Logger),
		rateLimiter.Middleware(), // Add rate limiting
		EnableCORS(),
	}

	mux.HandleFunc("/agent/task", s.Chain(s.handleAgentTask, standardStack...))
	mux.HandleFunc("/agent/approve", s.Chain(s.handleApprovalResponse, standardStack...))

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
	}, standardStack...))

	mux.HandleFunc("/health", s.Chain(s.handleHealth, EnableCORS()))

	s.Logger.Info(fmt.Sprintf("🚀 Agent Daemon listening on :%s", s.Port))
	return http.ListenAndServe(":"+s.Port, mux)
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
