package trace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/llm"
)

type LLMTrace struct {
	Timestamp time.Time                `json:"timestamp"`
	Messages  []llm.Message            `json:"messages"`
	Response  llm.Message              `json:"response"`
	Usage     domainContext.TokenUsage `json:"usage"`
	Error     string                   `json:"error,omitempty"`
}

type ToolTrace struct {
	Timestamp time.Time `json:"timestamp"`
	Name      string    `json:"name"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
}

type Transcript struct {
	SessionID  string      `json:"session_id"`
	StartTime  time.Time   `json:"start_time"`
	EndTime    time.Time   `json:"end_time"`
	LLMTraces  []LLMTrace  `json:"llm_traces"`
	ToolTraces []ToolTrace `json:"tool_traces"`
}

// Recorder handles appending to a session transcript safely across goroutines.
type Recorder struct {
	mu         sync.Mutex
	transcript Transcript
	outDir     string
}

func NewRecorder(sessionID, outDir string) *Recorder {
	_ = os.MkdirAll(outDir, 0755)
	return &Recorder{
		outDir: outDir,
		transcript: Transcript{
			SessionID:  sessionID,
			StartTime:  time.Now(),
			LLMTraces:  make([]LLMTrace, 0),
			ToolTraces: make([]ToolTrace, 0),
		},
	}
}

func (r *Recorder) RecordLLM(msgs []llm.Message, resp llm.Message, usage domainContext.TokenUsage, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errStr string
	if err != nil {
		errStr = err.Error()
	}

	r.transcript.LLMTraces = append(r.transcript.LLMTraces, LLMTrace{
		Timestamp: time.Now(),
		Messages:  msgs,
		Response:  resp,
		Usage:     usage,
		Error:     errStr,
	})
	r.flush()
}

func (r *Recorder) RecordTool(name, input, output string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errStr string
	if err != nil {
		errStr = err.Error()
	}

	r.transcript.ToolTraces = append(r.transcript.ToolTraces, ToolTrace{
		Timestamp: time.Now(),
		Name:      name,
		Input:     input,
		Output:    output,
		Error:     errStr,
	})
	r.flush()
}

func (r *Recorder) Finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transcript.EndTime = time.Now()
	r.flush()
}

func (r *Recorder) flush() {
	data, _ := json.MarshalIndent(r.transcript, "", "  ")
	path := filepath.Join(r.outDir, "transcript_"+r.transcript.SessionID+".json")
	_ = os.WriteFile(path, data, 0644)
}

// ReplayState manages deterministic playback of a loaded transcript.
type ReplayState struct {
	transcript *Transcript
	llmIndex   int
	toolIndex  int
	mu         sync.Mutex
}

func LoadReplayState(path string) (*ReplayState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Transcript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &ReplayState{transcript: &t}, nil
}

func (rs *ReplayState) NextLLM() (llm.Message, domainContext.TokenUsage, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.llmIndex >= len(rs.transcript.LLMTraces) {
		return llm.Message{}, domainContext.TokenUsage{}, errors.New("replay: no more LLM traces available")
	}

	trace := rs.transcript.LLMTraces[rs.llmIndex]
	rs.llmIndex++

	var err error
	if trace.Error != "" {
		err = errors.New(trace.Error)
	}

	return trace.Response, trace.Usage, err
}

func (rs *ReplayState) NextTool(name string) (string, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.toolIndex >= len(rs.transcript.ToolTraces) {
		return "", fmt.Errorf("replay: no more Tool traces available for %s", name)
	}

	trace := rs.transcript.ToolTraces[rs.toolIndex]
	rs.toolIndex++

	if trace.Name != name {
		return "", fmt.Errorf("replay desync: expected tool %s, but transcript had %s", name, trace.Name)
	}

	var err error
	if trace.Error != "" {
		err = errors.New(trace.Error)
	}

	return trace.Output, err
}