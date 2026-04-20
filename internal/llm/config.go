package llm

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// LLMConfig holds all user-configurable LLM settings.
type LLMConfig struct {
	Provider string `toml:"provider"` // "ollama" (default) | "openai"
	Model    string `toml:"model"`
	APIKey   string `toml:"api_key"` // required for cloud providers only
	URL      string `toml:"url"`     // override base URL; leave empty for provider default
}

// fileConfig is the top-level TOML structure.
type fileConfig struct {
	LLM LLMConfig `toml:"llm"`
}

// DefaultConfig returns sensible defaults: Ollama running locally.
func DefaultConfig() LLMConfig {
	return LLMConfig{
		Provider: "ollama",
		Model:    "llama3.2",
		URL:      "http://localhost:11434",
	}
}

// LoadConfig reads from the standard config path ~/.config/gameperf/config.toml.
// If the file does not exist, DefaultConfig is returned without error.
func LoadConfig() (LLMConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig(), nil
	}
	return LoadConfigFrom(home + "/.config/gameperf/config.toml")
}

// LoadConfigFrom reads from an explicit path. Missing file returns DefaultConfig.
// Exposed for testing.
func LoadConfigFrom(path string) (LLMConfig, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	var fc fileConfig
	if _, err := toml.Decode(string(data), &fc); err != nil {
		return cfg, fmt.Errorf("config parse error: %w", err)
	}

	// Only override defaults for non-empty values in the file.
	if fc.LLM.Provider != "" {
		cfg.Provider = fc.LLM.Provider
	}
	if fc.LLM.Model != "" {
		cfg.Model = fc.LLM.Model
	}
	if fc.LLM.APIKey != "" {
		cfg.APIKey = fc.LLM.APIKey
	}
	if fc.LLM.URL != "" {
		cfg.URL = fc.LLM.URL
	}

	return cfg, nil
}

// NewFromConfig creates the appropriate Provider from config.
// Unknown provider names fall back to Ollama.
func NewFromConfig(cfg LLMConfig) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.Model, cfg.APIKey, cfg.URL), nil
	case "gemini":
		return NewGeminiProvider(cfg.Model, cfg.APIKey, cfg.URL), nil
	default: // "ollama" or anything unknown
		url := cfg.URL
		if url == "" {
			url = "http://localhost:11434"
		}
		return NewOllamaProvider(cfg.Model, url), nil
	}
}
