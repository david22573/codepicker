package agent

// AgentResult standardizes the output from any agent execution
type AgentResult struct {
	Agent   AgentType
	Task    string
	Output  string
	Success bool
}
