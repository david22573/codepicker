Roadmap
Phase 1: Foundation & Quality (Weeks 1-3)
Task	Priority	Effort	Description
Comprehensive Testing	P0	High	Add unit tests for agent, server, openrouter, shadow
Config Consolidation	P0	Low	Use constants.DefaultModel everywhere, validate config on startup
Documentation	P0	Medium	README, inline docs, API docs for server endpoints
Error Handling Audit	P1	Medium	Remove silent ignores, add context to all errors

# Example: Test coverage targets
go test ./internal/agent/... -cover  # Target: 80%+
go test ./internal/server/... -cover # Target: 75%+
Phase 2: Robustness & Security (Weeks 4-6)
Task	Priority	Effort	Description
CORS Configuration	P0	Low	Make origins configurable via env/config
Request Body Limits	P1	Low	Add http.MaxBytesReader to all handlers
Memory Bounds	P1	Medium	LRU eviction for WorkingMemory, max context size
Shadow File Limits	P1	Low	Size limits, count limits per session
Audit Logging	P2	Medium	Log all tool executions and file modifications

// Example: Configurable CORS
type ServerConfig struct {
    AllowedOrigins []string `yaml:"allowed_origins"`
}

func EnableCORS(origins []string) Middleware {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            if slices.Contains(origins, origin) || slices.Contains(origins, "*") {
                w.Header().Set("Access-Control-Allow-Origin", origin)
            }
            // ...
        }
    }
}
Phase 3: Features & UX (Weeks 7-10)
Task	Priority	Effort	Description
Multi-Provider AI	P1	High	Abstract AI client, support Anthropic/OpenAI directly
Dynamic Pricing	P2	Medium	Fetch model pricing from OpenRouter API
Apply Command	P1	Low	codepicker apply to commit shadow changes
Conversation Persistence	P2	Medium	Save/load chat history
Enhanced Git Diff	P2	Medium	--diff staged, --diff branch1..branch2

// Example: Apply command
var applyCmd = &cobra.Command{
    Use:   "apply [path]",
    Short: "Apply shadow filesystem changes to the real filesystem",
    RunE: func(cmd *cobra.Command, args []string) error {
        sm, _ := shadow.NewManager(srcDir)
        files, _ := sm.ListShadowFiles()
        
        for _, f := range files {
            diff, _ := sm.PreviewDiff(f)
            fmt.Println(diff)
            if confirm("Apply this change?") {
                sm.Apply(f)
            }
        }
        return nil
    },
}
Phase 4: Production & Observability (Weeks 11-14)
Task	Priority	Effort	Description
Structured Logging	P1	Medium	JSON logs with fields, not string formatting
OpenTelemetry	P2	High	Distributed tracing for agent tasks
Metrics Endpoint	P2	Medium	Prometheus /metrics for server mode
Docker/K8s	P2	Medium	Multi-arch images, Helm chart
Graceful Shutdown	P1	Low	Handle SIGTERM, drain connections

// Example: Structured Logger
type StructuredLogger struct {
    enc *json.Encoder
}

func (l *StructuredLogger) Info(msg string, fields ...Field) {
    l.enc.Encode(LogEntry{
        Level:     "info",
        Message:   msg,
        Timestamp: time.Now(),
        Fields:    fieldsToMap(fields),
    })
}
Phase 5: Ecosystem (Weeks 15+)
Task	Priority	Effort	Description
VS Code Extension	P3	High	Context generation, inline ask
GitHub Action	P2	Medium	codepicker/action@v1 for CI
Plugin System	P3	Very High	Custom tools, minifiers
Documentation Site	P2	Medium	Examples, tutorials, API reference
Quick Wins (1-2 Days Each)
#	Task	File(s)	Impact
1	Use constants.DefaultModel everywhere	serve.go, do.go	Consistency
2	Log .env load failures	root.go	Debuggability
3	Add --dry-run flag	root.go	UX
4	Make CORS configurable	middleware.go, limits.go	Security
5	Add request body limit	server.go	Security
6	Write README.md	New file	Adoption
7	Add WorkingMemory size limit	memory.go	Stability
Suggested Immediate Actions

# 1. Fix model consistency
sed -i 's/"xiaomi\/mimo-v2-flash:free"/constants.DefaultModel/g' cmd/*.go

# 2. Add basic server tests
touch internal/server/handlers_test.go
touch internal/agent/engine_test.go

# 3. Create README template
cat > README.md << 'EOF'
# codepicker

Harvest code for AI consumption.

## Installation
```bash
go install github.com/david22573/codepicker@latest
Quick Start

codepicker init
codepicker --src ./myproject --out context.md
codepicker ask "Explain the main function"
EOF



---

## Architecture Recommendations

### 1. Dependency Injection for Testing
```go
// Current: Hard dependency on OpenRouter
eng, _ := agent.NewEngine(client, model, ...)

// Better: Interface injection
type AIProvider interface {
    Chat(ctx context.Context, msgs []Message) (*Response, error)
}

func NewEngine(ai AIProvider, ...) *Engine { ... }
2. Unified Command Options

// Current: Duplicated validation in each command
apiKey, err := validateAPIKey()  // ask.go
apiKey, err := validateAPIKey()  // chat.go

// Better: Shared pre-run hook
var aiCmd = &cobra.Command{
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return initAIContext(cmd) // Validates key, loads config
    },
}
3. Context Generation as Service

// Current: contextgen.Generate creates temp file, writes, reads back

// Better: Stream-based approach
type ContextBuilder struct {
    w io.Writer
}

func (cb *ContextBuilder) AddFile(path string, content []byte) error
func (cb *ContextBuilder) Build() string
Summary
codepicker is a solid foundation with thoughtful architecture. The main areas requiring attention are:

Testing - Critical gap that blocks confident refactoring
Configuration - Consolidate to prevent drift
Documentation - Essential for adoption
Memory Safety - Bound context growth for long-running agents
The roadmap prioritizes stability and security before features, ensuring the tool remains reliable as it grows.

