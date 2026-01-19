package agents

type AgentType string

const (
	AgentOrchestrator AgentType = "Orchestrator"
	AgentContext      AgentType = "Context"
	AgentModifier     AgentType = "CodeModifier"
	AgentSystem       AgentType = "System"
	AgentQuality      AgentType = "Quality"
)

// AgentResult standardizes the output from any agent execution
type AgentResult struct {
	Agent   AgentType
	Task    string
	Output  string
	Success bool
}
