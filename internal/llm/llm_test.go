package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/collector"
	"github.com/zenardi/gameperf/internal/llm"
	"github.com/zenardi/gameperf/internal/report"
)

// ── Mock provider ─────────────────────────────────────────────────────────────

type mockProvider struct {
	name       string
	response   string
	err        error
	called     bool
	lastPrompt string
}

func (m *mockProvider) Complete(_ context.Context, prompt string) (string, error) {
	m.called = true
	m.lastPrompt = prompt
	return m.response, m.err
}
func (m *mockProvider) Name() string { return m.name }

// ── Test fixtures ─────────────────────────────────────────────────────────────

func makeReport() report.FullReport {
	return report.FullReport{
		GeneratedAt: time.Now(),
		Snapshot: analyzer.Snapshot{
			GPU: collector.GPUStat{MemoryUsed: 7000, MemoryTotal: 8192},
			MemInfo: collector.MemInfo{
				MemTotal:     16 * 1024 * 1024,
				MemAvailable: 2 * 1024 * 1024,
				SwapTotal:    4 * 1024 * 1024,
				SwapFree:     500 * 1024,
			},
			VMMaxMapCount: 65530,
			THPMode:       "always",
		},
		Findings: []analyzer.Finding{
			{ID: "cpu-governor", Severity: analyzer.SeverityCritical, Title: "P-cores on powersave"},
			{ID: "vram-pressure", Severity: analyzer.SeverityWarning, Title: "VRAM near full", Evidence: "7000/8192 MiB"},
		},
	}
}

// ── BuildPrompt ───────────────────────────────────────────────────────────────

func TestBuildPrompt_ContainsFindings(t *testing.T) {
	t.Parallel()
	prompt := llm.BuildPrompt(makeReport())
	for _, wantID := range []string{"cpu-governor", "vram-pressure"} {
		if !strings.Contains(prompt, wantID) {
			t.Errorf("BuildPrompt missing finding ID %q", wantID)
		}
	}
}

func TestBuildPrompt_ContainsSnapshotMetrics(t *testing.T) {
	t.Parallel()
	prompt := llm.BuildPrompt(makeReport())
	for _, want := range []string{"7000", "8192", "vm.max_map_count", "THP"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildPrompt missing %q in prompt", want)
		}
	}
}

func TestBuildPrompt_ContainsSeverity(t *testing.T) {
	t.Parallel()
	prompt := llm.BuildPrompt(makeReport())
	if !strings.Contains(strings.ToLower(prompt), "critical") {
		t.Error("BuildPrompt should include severity labels")
	}
}

func TestBuildPrompt_NoFindings_SaysNone(t *testing.T) {
	t.Parallel()
	r := makeReport()
	r.Findings = nil
	prompt := llm.BuildPrompt(r)
	if !strings.Contains(prompt, "No issues") {
		t.Error("BuildPrompt should say 'No issues' when findings list is empty")
	}
}

// ── EnhanceReport ─────────────────────────────────────────────────────────────

func TestEnhanceReport_CallsProvider(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{name: "mock", response: "AI says: disable Frame Generation"}
	result, err := llm.EnhanceReport(context.Background(), mock, makeReport())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("EnhanceReport did not call the provider")
	}
	if result != "AI says: disable Frame Generation" {
		t.Errorf("got %q, want provider response verbatim", result)
	}
}

func TestEnhanceReport_PromptContainsFindings(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{name: "mock", response: "ok"}
	_, _ = llm.EnhanceReport(context.Background(), mock, makeReport())
	if !strings.Contains(mock.lastPrompt, "cpu-governor") {
		t.Error("prompt sent to provider must contain finding IDs")
	}
}

func TestEnhanceReport_PropagatesError(t *testing.T) {
	t.Parallel()
	mock := &mockProvider{name: "mock-prov", err: errors.New("timeout")}
	_, err := llm.EnhanceReport(context.Background(), mock, makeReport())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mock-prov") {
		t.Errorf("error should name the provider, got: %v", err)
	}
}

// ── OllamaProvider ────────────────────────────────────────────────────────────

func TestOllamaProvider_Name(t *testing.T) {
	t.Parallel()
	p := llm.NewOllamaProvider("llama3.2", "http://localhost:11434")
	if !strings.Contains(p.Name(), "ollama") {
		t.Errorf("Name() = %q, want 'ollama'", p.Name())
	}
	if !strings.Contains(p.Name(), "llama3.2") {
		t.Errorf("Name() = %q, want model name", p.Name())
	}
}

func TestOllamaProvider_Complete(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type: application/json")
		}
		json.NewEncoder(w).Encode(map[string]string{"response": "ollama says hello"})
	}))
	defer srv.Close()

	p := llm.NewOllamaProvider("llama3.2", srv.URL)
	got, err := p.Complete(context.Background(), "diagnose my game")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "ollama says hello") {
		t.Errorf("got %q, want 'ollama says hello'", got)
	}
}

func TestOllamaProvider_Complete_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := llm.NewOllamaProvider("bad-model", srv.URL)
	_, err := p.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestOllamaProvider_Complete_OllamaErrorField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"error": "model not found"})
	}))
	defer srv.Close()

	p := llm.NewOllamaProvider("missing-model", srv.URL)
	_, err := p.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when response contains error field")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error should contain ollama error message, got: %v", err)
	}
}

// ── OpenAIProvider ────────────────────────────────────────────────────────────

func TestOpenAIProvider_Name(t *testing.T) {
	t.Parallel()
	p := llm.NewOpenAIProvider("gpt-4o-mini", "sk-test", "")
	if !strings.Contains(p.Name(), "openai") {
		t.Errorf("Name() = %q, want 'openai'", p.Name())
	}
	if !strings.Contains(p.Name(), "gpt-4o-mini") {
		t.Errorf("Name() = %q, want model name", p.Name())
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("wrong auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "openai says hi"}},
			},
		})
	}))
	defer srv.Close()

	p := llm.NewOpenAIProvider("gpt-4o-mini", "sk-test", srv.URL)
	got, err := p.Complete(context.Background(), "analyse this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "openai says hi") {
		t.Errorf("got %q, want 'openai says hi'", got)
	}
}

func TestOpenAIProvider_Complete_APIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "invalid api key"},
		})
	}))
	defer srv.Close()

	p := llm.NewOpenAIProvider("gpt-4o-mini", "bad-key", srv.URL)
	_, err := p.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error on 401 response")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error should contain API message, got: %v", err)
	}
}

func TestOpenAIProvider_Complete_EmptyChoices(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	p := llm.NewOpenAIProvider("gpt-4o-mini", "sk-x", srv.URL)
	_, err := p.Complete(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when choices is empty")
	}
}

// ── Config ────────────────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := llm.DefaultConfig()
	if cfg.Provider != "ollama" {
		t.Errorf("default provider = %q, want 'ollama'", cfg.Provider)
	}
	if cfg.Model == "" {
		t.Error("default model should not be empty")
	}
	if cfg.URL == "" {
		t.Error("default URL should not be empty")
	}
}

func TestLoadConfigFrom_ReturnsDefaults_WhenNoFile(t *testing.T) {
	t.Parallel()
	cfg, err := llm.LoadConfigFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Errorf("provider = %q, want 'ollama'", cfg.Provider)
	}
}

func TestLoadConfigFrom_OverridesDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(path, []byte(`
[llm]
provider = "openai"
model    = "gpt-4o"
api_key  = "sk-abc"
url      = "https://custom.openai.com"
`), 0600)

	cfg, err := llm.LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != "openai" {
		t.Errorf("provider = %q, want 'openai'", cfg.Provider)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("model = %q, want 'gpt-4o'", cfg.Model)
	}
	if cfg.APIKey != "sk-abc" {
		t.Errorf("api_key = %q, want 'sk-abc'", cfg.APIKey)
	}
	if cfg.URL != "https://custom.openai.com" {
		t.Errorf("url = %q, want custom URL", cfg.URL)
	}
}

func TestLoadConfigFrom_InvalidTOML_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(path, []byte(`[llm`), 0600) // malformed TOML

	_, err := llm.LoadConfigFrom(path)
	if err == nil {
		t.Fatal("expected error for malformed TOML")
	}
}

func TestNewFromConfig_Ollama(t *testing.T) {
	t.Parallel()
	cfg := llm.LLMConfig{Provider: "ollama", Model: "mistral", URL: "http://localhost:11434"}
	p, err := llm.NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p.Name(), "ollama") {
		t.Errorf("expected ollama provider, got %q", p.Name())
	}
}

func TestNewFromConfig_OpenAI(t *testing.T) {
	t.Parallel()
	cfg := llm.LLMConfig{Provider: "openai", Model: "gpt-4o-mini", APIKey: "sk-x"}
	p, err := llm.NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p.Name(), "openai") {
		t.Errorf("expected openai provider, got %q", p.Name())
	}
}

func TestNewFromConfig_UnknownProvider_FallsBackToOllama(t *testing.T) {
	t.Parallel()
	cfg := llm.LLMConfig{Provider: "fictional-llm", Model: "x"}
	p, err := llm.NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unknown providers fall back to Ollama.
	if !strings.Contains(p.Name(), "ollama") {
		t.Errorf("unknown provider should fall back to ollama, got %q", p.Name())
	}
}

// suppress unused import lint in case fmt is only used in error formatting
var _ = fmt.Sprintf
