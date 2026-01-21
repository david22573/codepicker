package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/david22573/codepicker/internal/agent"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type PlanModel struct {
	table    table.Model
	plan     *agent.ExecutionPlan
	selected map[int]bool // Key is index in plan.Steps
	quitting bool
	approved bool
}

func ReviewPlan(plan *agent.ExecutionPlan) bool {
	columns := []table.Column{
		{Title: "Run?", Width: 6},
		{Title: "Agent", Width: 12},
		{Title: "Task", Width: 60},
	}

	rows := []table.Row{}
	for _, step := range plan.Steps {
		rows = append(rows, table.Row{
			"[x]", // Default checked
			string(step.Agent),
			step.Task,
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	m := PlanModel{
		table:    t,
		plan:     plan,
		selected: make(map[int]bool),
		approved: false,
	}

	// Default all to true
	for i := range plan.Steps {
		m.selected[i] = true
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error running plan review: %v\n", err)
		return false
	}

	final := finalModel.(PlanModel)
	if !final.approved {
		return false
	}

	// Filter the plan based on selection
	var newSteps []agent.PlanStep
	for i, step := range plan.Steps {
		if final.selected[i] {
			newSteps = append(newSteps, step)
		}
	}
	plan.Steps = newSteps
	return true
}

func (m PlanModel) Init() tea.Cmd { return nil }

func (m PlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.approved = true
			return m, tea.Quit
		case " ":
			// Toggle selection
			idx := m.table.Cursor()
			m.selected[idx] = !m.selected[idx]

			// Update visual row
			rows := m.table.Rows()
			mark := "[ ]"
			if m.selected[idx] {
				mark = "[x]"
			}
			rows[idx][0] = mark
			m.table.SetRows(rows)
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m PlanModel) View() string {
	if m.quitting {
		return "Plan cancelled.\n"
	}
	if m.approved {
		return ""
	}
	return baseStyle.Render(m.table.View()) + "\n  [Space] Toggle  [Enter] Confirm  [q] Cancel\n"
}
