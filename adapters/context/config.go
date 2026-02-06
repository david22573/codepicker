package context

type Config struct {
	ProjectRoot     string
	MaxTokens       int
	IncludePatterns []string
	ExcludePatterns []string
	TaskDescription string
}
