package agent

import "time"

type SessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	ID        string           `json:"id"`
	Task      string           `json:"task"`
	CreatedAt time.Time        `json:"created_at"`
	Messages  []SessionMessage `json:"messages"`
	EditsMade []string         `json:"edits_made"`
	Outcome   string           `json:"outcome"`
}

type Learning struct {
	ID        string    `json:"id"`
	Task      string    `json:"task"`
	Note      string    `json:"note"`
	Embedding []float32 `json:"embedding"`
	CreatedAt time.Time `json:"created_at"`
}
