# gameperf — Copilot Instructions

## Project overview

`gameperf` is a Linux game performance diagnostics and auto-fix tool written in Go.
It collects real system metrics (CPU governor, IRQ affinity, VRAM, RAM, swap, THP),
detects issues via a rule engine, and produces reports in console/Markdown/JSON formats.
Target platform: Linux x86-64. Primary use case: Steam/Proton gaming on hybrid-CPU laptops.

Module path: `github.com/zenardi/gameperf`
Go version: 1.24+
Only external dependency: `github.com/spf13/cobra` (CLI)

---

## Test-driven development — mandatory workflow

**Always write the test before the implementation.** Every feature, collector, rule, or
formatter must follow this order:

1. **Write the test first** — define the expected behaviour in a `_test.go` file
2. **Run it and confirm it fails** — `go test ./...` must show a compile error or `FAIL`
3. **Write the minimum implementation to make it pass**
4. **Run tests again** — all must be green before committing

Never write implementation code without a corresponding test already in place.
If you are asked to add a feature without a test, write the test first and flag it.

---

## Package layout and responsibilities

```
cmd/gameperf/main.go          CLI entry point — cobra subcommands only, no business logic
internal/collector/           Low-level metric readers from /proc and /sys
internal/analyzer/            Diagnostic rule engine — pure functions, no I/O
internal/fixer/               Executes auto-fix shell commands
internal/report/              Output formatters (console, Markdown, JSON)
internal/llm/                 LLM provider abstraction and report enhancement (planned)
```

---

## Collector conventions

### Testability pattern — always follow this

Every collector that reads a file must expose two functions:

```go
// Collect* — opens the real file and delegates to Parse*
func CollectFoo() (Foo, error) {
    f, err := os.Open("/proc/foo")
    if err != nil { return Foo{}, err }
    defer f.Close()
    return ParseFoo(f)
}

// Parse* — accepts io.Reader; NO filesystem access; exported for tests
func ParseFoo(r io.Reader) (Foo, error) { ... }
```

For values that are a single string (sysctl-like files), the pattern is:

```go
func CollectBar() (string, error) {
    data, err := os.ReadFile("/proc/sys/bar")
    if err != nil { return "", err }
    return ParseBar(string(data)), nil
}

// ParseBar takes the raw file content as a string
func ParseBar(s string) string { return strings.TrimSpace(s) }
```

For sysfs trees (multiple files per CPU), expose an internal helper:

```go
func CollectCPUFoo() ([]CPUFoo, error) {
    return collectCPUFooFrom("/sys/devices/system/cpu")
}

// collectCPUFooFrom accepts sysfsRoot for test injection
func collectCPUFooFrom(sysfsRoot string) ([]CPUFoo, error) { ... }
```

### Rules
- `Collect*` functions are **never** called in tests — tests call `Parse*` only
- `Parse*` functions must never open files, run subprocesses, or have side effects
- GPU output is parsed from a raw CSV string: `ParseGPUOutput(string) (GPUStat, error)`

---

## Analyzer conventions

### Snapshot

`Snapshot` is the single struct passed to all rules. It is populated by `Collect()` and
contains pre-collected metric values — rules never collect data themselves.

### Rule functions

Each rule is a private function with this signature:

```go
func checkSomething(snap Snapshot) []Finding { ... }
```

- Pure function: same input → same output, no I/O, no global state
- Returns `nil` (not empty slice) when no issue is found
- Returns one `Finding` per distinct problem instance (e.g., one per bad IRQ)
- All rules are wired into `Analyze()` — nowhere else

### Finding fields

```go
Finding{
    ID:          "kebab-case-unique-id",   // used in tests and reports
    Severity:    SeverityCritical | SeverityWarning | SeverityInfo,
    Title:       "Short one-line description (shown in console)",
    Description: "Why this is a problem, what causes it",
    Evidence:    "Raw metric values that triggered this finding (omitempty)",
    AutoFixable: true/false,
    AutoFixCmd:  []string{...},  // only set when AutoFixable = true
    ManualFix:   "Shell commands / steps for manual fix",
    InGameFix:   "Steps inside game settings (optional)",
}
```

**Never** set `AutoFixCmd` on a finding unless the command is safe, deterministic,
and reversible. LLM-generated commands must never populate `AutoFixCmd`.

---

## Testing conventions

### File naming
- Test file lives next to the file under test: `irq.go` → `irq_test.go`
- Package: `package collector_test` (black-box) for collector and report tests
- Package: `package analyzer_test` for analyzer tests

### Test naming
```
Test<Type>_<Scenario>_<ExpectedOutcome>
TestParseIRQs_NvidiaEntry_TotalIsSum
TestAnalyze_IRQOnECore_ProducesCriticalFinding
TestAnalyze_CPUGovernor_Powersave_Critical
```

### Synthetic data — never use real /proc files
```go
const procInterrupts4CPU = `           CPU0       CPU1 ...
217:     116647       0  ...  nvidia
`
// Pass to ParseIRQs(strings.NewReader(procInterrupts4CPU))
```

### Snapshot builders for analyzer tests

Use functional option helpers — do not construct `Snapshot` inline in each test:

```go
func buildSnapshot(opts ...func(*analyzer.Snapshot)) analyzer.Snapshot {
    snap := analyzer.Snapshot{
        CPUTopology: defaultTopology, // 2 P-cores + 2 E-cores
    }
    for _, o := range opts { o(&snap) }
    return snap
}

func withNvidiaIRQOnECore() func(*analyzer.Snapshot) {
    return func(s *analyzer.Snapshot) { ... }
}
```

### Edge cases to always cover
For every new rule or parser, include tests for:
- Happy path (issue detected)
- Negative path (no issue — make sure the rule is silent)
- Zero/empty input (no panic, sensible default)
- Boundary values (e.g., exactly at threshold)

### Report tests
- Use `report.FullReport` (not `analyzer.Report`) as input to formatters
- Test that output *contains* expected strings — do not assert full output equality
- JSON tests decode the output and assert field presence and type

---

## Code style

- Comments only on exported symbols and non-obvious logic — no filler comments
- No global mutable state outside `cmd/`
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Optional sysfs reads (CPU freq, THP, governors) silently skip on error —
  they are enrichment, not required for the tool to function
- Struct field json tags use `snake_case` and `omitempty` for optional fields:
  ```go
  AutoFixCmd []string `json:"auto_fix_cmd,omitempty"`
  ```
- Sort results by ID when returning slices (CPUGovernors, CPUFreqs) for determinism

---

## CLI conventions

- Business logic never lives in `cmd/` — only flag wiring and `run*` functions
- `run*` functions call `analyzer.Collect()` → `analyzer.Analyze()` → `report.Write*()`
- New subcommands follow the existing pattern in `main.go`
- The `report` command defaults `--format` to `markdown`; `diagnose`/`monitor` default to `console`
- `--llm` flag (planned) is always opt-in, never default

---

## LLM integration (planned — `internal/llm/`)

When implementing:
- Define `Provider` interface first, write mock provider, write tests against mock
- `EnhanceReport()` accepts `FullReport`, returns Markdown string — pure function once provider is injected
- LLM output is always appended to the report as an "AI Analysis" section — never replaces rule findings
- LLM output never generates `AutoFixCmd` values — suggestions are display-only
- Ollama (local) is the default provider — no API key, no data leaves the machine
- Cloud providers (OpenAI, Anthropic) are opt-in via config or `GAMEPERF_LLM_KEY` env var

---

## Build and test

```bash
make build    # go build -o dist/gameperf ./cmd/gameperf
make test     # go test ./...
make install  # go install ./cmd/gameperf
make clean    # rm -rf dist/
```

The test suite must pass with no network access and no real `/proc`/`/sys` access.
All tests use synthetic data injected via `io.Reader`, string parameters, or temp directories.
