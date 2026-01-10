package minifier

import (
	"strings"
	"sync"
)

// Strategy defines the interface for language-specific minifiers
type Strategy interface {
	Minify(content []byte) []byte
}

var (
	defaultRegistry *Registry
	once            sync.Once
)

// Registry maps file extensions to minification strategies
type Registry struct {
	strategies map[string]Strategy
	fallback   Strategy
}

func GetRegistry() *Registry {
	once.Do(func() {
		defaultRegistry = &Registry{
			strategies: make(map[string]Strategy),
			fallback:   &GenericMinifier{},
		}
		// Register default strategies
		defaultRegistry.Register(&GoMinifier{}, ".go")
		defaultRegistry.Register(&JSMinifier{}, ".js", ".ts", ".tsx", ".jsx")
		defaultRegistry.Register(&PythonMinifier{}, ".py")
		defaultRegistry.Register(&PassthroughMinifier{}, ".json", ".yaml", ".yml", ".toml", ".xml", ".md", ".txt")
	})
	return defaultRegistry
}

func (r *Registry) Register(s Strategy, exts ...string) {
	for _, ext := range exts {
		r.strategies[ext] = s
	}
}

// Minify is the main entry point used by writer.go.
// It delegates to the appropriate strategy based on file extension.
func Minify(content []byte, ext string) []byte {
	r := GetRegistry()
	ext = strings.ToLower(ext)

	strategy, exists := r.strategies[ext]
	if !exists {
		strategy = r.fallback
	}

	minified := strategy.Minify(content)
	return SqueezeVerticalWhitespace(minified)
}

