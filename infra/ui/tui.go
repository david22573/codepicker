package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styles ---

var (
	TitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Padding(0, 1)
	InfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#33CCFF"))
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	BoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).BorderForeground(lipgloss.Color("#888888"))

	DiffAddStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	DiffSubStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	DiffHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Bold(true)
)

// --- Bubble Tea Spinner Model ---

type taskResultMsg struct{ err error }

type spinnerModel struct {
	spinner  spinner.Model
	text     string
	action   func() error
	err      error
	quitting bool
}

func newSpinnerModel(text string, action func() error) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return spinnerModel{
		spinner: s,
		text:    text,
		action:  action,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		// Run the action in a goroutine and send a message when done
		func() tea.Msg {
			err := m.action()
			return taskResultMsg{err}
		},
	)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = fmt.Errorf("interrupted by user")
			m.quitting = true
			return m, tea.Quit
		}
	case taskResultMsg:
		m.err = msg.err
		m.quitting = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.quitting {
		return ""
	}
	return fmt.Sprintf("\n %s %s\n\n", m.spinner.View(), m.text)
}

// --- Public Helper ---

// RunSpinner executes a blocking function while showing a Bubble Tea spinner.
func RunSpinner(text string, action func() error) error {
	p := tea.NewProgram(newSpinnerModel(text, action))
	m, err := p.Run()
	if err != nil {
		return err
	}
	// Check if the model caught an internal error from the action
	if finalModel, ok := m.(spinnerModel); ok {
		return finalModel.err
	}
	return nil
}

// --- Diff Renderer ---

// RenderDiff formats the raw search/replace blocks into a readable unified diff.
func RenderDiff(filename, blocks string) string {
	var sb strings.Builder
	sb.WriteString("\n" + DiffHeaderStyle.Render(fmt.Sprintf("--- %s (Pending Change)", filename)) + "\n")

	lines := strings.Split(blocks, "\n")
	inOrig := false
	inRep := false

	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<") {
			inOrig = true
			inRep = false
			continue
		}
		if strings.HasPrefix(line, "====") {
			inOrig = false
			inRep = true
			continue
		}
		if strings.HasPrefix(line, ">>>>") {
			inOrig = false
			inRep = false
			continue
		}

		if inOrig {
			sb.WriteString(DiffSubStyle.Render("- "+line) + "\n")
		} else if inRep {
			sb.WriteString(DiffAddStyle.Render("+ "+line) + "\n")
		} else {
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// --- Log Helpers ---

func PrintHeader(title string) {
	fmt.Println(TitleStyle.Render("\n=== " + title + " ==="))
}

func PrintSuccess(msg string) {
	fmt.Println(SuccessStyle.Render("✔ " + msg))
}

func PrintError(msg string) {
	fmt.Println(ErrorStyle.Render("✖ " + msg))
}

func PrintInfo(msg string) {
	fmt.Println(InfoStyle.Render("ℹ " + msg))
}

func PrintWarning(msg string) {
	fmt.Println(WarningStyle.Render("⚠ " + msg))
}
