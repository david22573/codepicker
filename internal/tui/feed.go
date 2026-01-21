package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	toolStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	resultStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
)

// AgentEvent is the generic message sent from the backend to the UI
type AgentEvent struct {
	Type    string // "thought", "tool_start", "tool_end", "step", "error"
	Content string
}

type FeedModel struct {
	spinner   spinner.Model
	viewport  viewport.Model
	messages  []string
	status    string
	width     int
	height    int
	quitting  bool
	EventChan chan AgentEvent
	DoneChan  chan struct{}
}

func NewFeedModel() FeedModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return FeedModel{
		spinner:   s,
		EventChan: make(chan AgentEvent, 100),
		DoneChan:  make(chan struct{}),
		messages:  []string{},
		status:    "Initializing agent...",
	}
}

func (m FeedModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.EventChan),
		waitForDone(m.DoneChan),
	)
}

func (m FeedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 3 // Reserve space for spinner + status
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case AgentEvent:
		var cmd tea.Cmd

		// Update Status Bar
		if msg.Type == "thought" {
			m.status = fmt.Sprintf("Thinking: %s...", truncate(msg.Content, 60))
		} else if msg.Type == "tool_start" {
			m.status = fmt.Sprintf("Executing: %s", msg.Content)
		} else if msg.Type == "step" {
			m.status = fmt.Sprintf("Step: %s", msg.Content)
		}

		// Add to Log History
		logLine := formatEvent(msg)
		if logLine != "" {
			m.messages = append(m.messages, logLine)
			// Keep history manageable
			if len(m.messages) > 200 {
				m.messages = m.messages[len(m.messages)-100:]
			}
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()
		}

		// Continue listening
		cmd = waitForEvent(m.EventChan)
		return m, cmd

	case struct{}: // Done signal
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m FeedModel) View() string {
	if m.quitting {
		return ""
	}

	header := fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	return fmt.Sprintf("%s\n\n%s", header, m.viewport.View())
}

// Helpers

func waitForEvent(ch <-chan AgentEvent) tea.Cmd {
	return func() tea.Msg {
		if evt, ok := <-ch; ok {
			return evt
		}
		return nil
	}
}

func waitForDone(ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-ch
		return struct{}{}
	}
}

func formatEvent(e AgentEvent) string {
	switch e.Type {
	case "tool_start":
		return toolStyle.Render(fmt.Sprintf("  ⚙️  %s", e.Content))
	case "tool_end":
		return resultStyle.Render(fmt.Sprintf("  └─ Done (%s)", e.Content))
	case "error":
		return errorStyle.Render(fmt.Sprintf("  ❌ %s", e.Content))
	case "step":
		return stepStyle.Render(fmt.Sprintf("\n▶ %s", e.Content))
	}
	return ""
}

func truncate(s string, max int) string {
	clean := strings.ReplaceAll(s, "\n", " ")
	if len(clean) > max {
		return clean[:max] + "..."
	}
	return clean
}
