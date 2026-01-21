package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/david22573/codepicker/internal/shadow"
)

var (
	docStyle = lipgloss.NewStyle().Margin(1, 2)

	// Diff Styles
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")) // Soft Green
	diffRemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")) // Soft Red
	diffMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9399B2")).Faint(true)
	diffHdrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Bold(true)
)

type item struct {
	path  string
	title string
	desc  string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type Model struct {
	list          list.Model
	viewport      viewport.Model
	shadowMgr     *shadow.Manager
	files         []string
	selectedFile  string
	width, height int
	quitting      bool
	err           error
}

func NewReviewModel(sm *shadow.Manager, files []string) Model {
	items := make([]list.Item, len(files))
	for i, f := range files {
		items[i] = item{path: f, title: f, desc: "Pending Change"}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "📦 Code Review"
	l.SetShowHelp(false)

	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	return Model{
		list:      l,
		viewport:  vp,
		shadowMgr: sm,
		files:     files,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		listWidth := int(float64(m.width) * 0.3)
		vpWidth := m.width - listWidth - 6

		m.list.SetSize(listWidth, m.height-2)
		m.viewport.Width = vpWidth
		m.viewport.Height = m.height - 2

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if i, ok := m.list.SelectedItem().(item); ok {
				// FIX: Call ApplyAtomic instead of Apply
				if _, err := m.shadowMgr.ApplyAtomic(i.path); err != nil {
					m.err = err
				} else {
					return m, m.list.NewStatusMessage(fmt.Sprintf("✅ Applied %s", i.path))
				}
			}
		}
	}

	newList, cmd := m.list.Update(msg)
	m.list = newList
	cmds = append(cmds, cmd)

	if i, ok := m.list.SelectedItem().(item); ok && i.path != m.selectedFile {
		m.selectedFile = i.path
		rawDiff, _ := m.shadowMgr.PreviewDiff(i.path)
		styledDiff := colorizeDiff(rawDiff)
		m.viewport.SetContent(styledDiff)
	}

	newVp, cmd := m.viewport.Update(msg)
	m.viewport = newVp
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		docStyle.Render(m.list.View()),
		docStyle.Render(m.viewport.View()),
	)
}

func Run(sm *shadow.Manager, files []string) error {
	m := NewReviewModel(sm, files)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func colorizeDiff(raw string) string {
	lines := strings.Split(raw, "\n")
	var out strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			out.WriteString(diffHdrStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "@@") {
			out.WriteString(diffMetaStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "+") {
			out.WriteString(diffAddStyle.Render(line) + "\n")
		} else if strings.HasPrefix(line, "-") {
			out.WriteString(diffRemStyle.Render(line) + "\n")
		} else {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}
