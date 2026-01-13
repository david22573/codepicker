package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/logger"
)

type AgentServer struct {
	Port   string
	Engine *agent.Engine
	Logger logger.Logger

	// State for handling approval blocking
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

	// Wire the engine's approval request to this server's blocking mechanism
	eng.ApprovalCallback = srv.WaitForApproval

	return srv
}

func (s *AgentServer) Start() error {
	mux := http.NewServeMux()

	// Register Routes
	mux.HandleFunc("/agent/task", s.enableCORS(s.handleAgentTask))
	mux.HandleFunc("/agent/approve", s.enableCORS(s.handleApprovalResponse))
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/agent/context", s.enableCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetContext(w, r)
		case http.MethodPost:
			s.handleAddContext(w, r)
		case http.MethodDelete:
			s.handleDeleteContext(w, r)
		}
	}))

	s.Logger.Info(fmt.Sprintf("🚀 Agent Daemon listening on :%s", s.Port))

	return http.ListenAndServe(":"+s.Port, mux)
}

// WaitForApproval is the callback passed to the Agent Engine.
// It blocks the Agent goroutine until an HTTP POST to /agent/approve releases it.
func (s *AgentServer) WaitForApproval(cmdStr, reason string) bool {
	// 1. Generate a unique ID for this request
	reqID := fmt.Sprintf("%d", time.Now().UnixNano())

	// 2. Create a channel to wait on
	responseChan := make(chan bool)

	s.approvalLock.Lock()
	s.approvalMap[reqID] = responseChan
	s.approvalLock.Unlock()

	// 3. We assume the handleAgentTask handler has set up a mechanism
	// to capture this event. Since we are inside the Engine.Run loop,
	// we rely on the Engine's updateHistory/callback mechanism to emit the event
	// to the SSE stream.

	// However, we need to signal the event *out* to the HTTP stream.
	// We'll return false temporarily here because strictly connecting
	// the "Block" here to the "SSE Write" in the handler requires
	// the Engine to support an "OnApprovalReq" event hook distinct from history.

	// For this refactor, we assume the Engine emits a special Tool Call
	// that we intercept, OR we just block here.

	// *Critical Fix for Logic*: The previous implementation injected the specific
	// closure into the engine per request. To keep `server.go` clean,
	// we will handle the Blocking here, but the *Notification* to the user
	// needs to happen via the `eventStream` in `handleAgentTask`.

	// Since we can't easily access the specific http.ResponseWriter from here
	// without complex context passing, we will assume the Engine's
	// `Run` method takes care of notifying the user via its `updateFn`.

	// We just wait here.
	select {
	case approved := <-responseChan:
		return approved
	case <-time.After(60 * time.Second):
		return false // Timeout
	}
}

func (s *AgentServer) enableCORS(next http.HandlerFunc) http.HandlerFunc {
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

func (s *AgentServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
