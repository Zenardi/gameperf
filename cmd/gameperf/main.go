package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/fixer"
	"github.com/zenardi/gameperf/internal/llm"
	"github.com/zenardi/gameperf/internal/report"
)

var (
	flagGames    []string
	flagFormat   string
	flagOutput   string
	flagAutoFix  bool
	flagSudo     bool
	flagInterval int

	// LLM flags
	flagLLM         bool
	flagLLMProvider string
	flagLLMModel    string
)

var defaultGameNames = []string{} // auto-detection is tried first; this is the fallback name list

func main() {
	root := &cobra.Command{
		Use:   "gameperf",
		Short: "Real-time game performance diagnostics for Linux",
		Long: `gameperf monitors system metrics while a game is running,
identifies performance issues (IRQ routing, VRAM pressure, CPU bottlenecks),
and produces detailed reports with auto-fix support.`,
	}

	// --- diagnose (default) ---
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Run a one-shot analysis and print findings",
		RunE:  runDiagnose,
	}
	addCommonFlags(diagnoseCmd)
	diagnoseCmd.Flags().BoolVar(&flagAutoFix, "fix", false, "Automatically apply all safe fixes after diagnosing")
	diagnoseCmd.Flags().BoolVar(&flagSudo, "sudo", false, "Prepend sudo to fix commands that require root")
	addLLMFlags(diagnoseCmd)

	// --- fix ---
	fixCmd := &cobra.Command{
		Use:   "fix",
		Short: "Diagnose and apply all auto-fixable issues",
		RunE:  runFix,
	}
	addCommonFlags(fixCmd)
	fixCmd.Flags().BoolVar(&flagSudo, "sudo", false, "Prepend sudo to commands that require root")

	// --- monitor (continuous) ---
	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Continuously monitor and re-diagnose at an interval",
		RunE:  runMonitor,
	}
	addCommonFlags(monitorCmd)
	monitorCmd.Flags().IntVar(&flagInterval, "interval", 10, "Seconds between each diagnostic run")

	// --- report ---
	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Run analysis and write a full report to a file",
		RunE:  runReport,
	}
	// report has its own --format flag: default is markdown, not console.
	reportCmd.Flags().StringSliceVar(&flagGames, "game", defaultGameNames, "Override auto-detected game (process name substrings); auto-detection used when empty")
	reportCmd.Flags().StringVar(&flagFormat, "format", "markdown", "Output format: console, markdown, json")
	reportCmd.Flags().StringVar(&flagOutput, "output", "gameperf-report.md", "Output file path")
	addLLMFlags(reportCmd)

	root.AddCommand(diagnoseCmd, fixCmd, monitorCmd, reportCmd, newServeCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&flagGames, "game", defaultGameNames, "Override auto-detected game (process name substrings); auto-detection used when empty")
	cmd.Flags().StringVar(&flagFormat, "format", "console", "Output format: console, markdown, json")
}

func addLLMFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&flagLLM, "llm", false, "Enhance output with an LLM analysis (requires Ollama or an API key)")
	cmd.Flags().StringVar(&flagLLMProvider, "llm-provider", "", "Override LLM provider: ollama, openai (default: from config)")
	cmd.Flags().StringVar(&flagLLMModel, "llm-model", "", "Override LLM model name (default: from config)")
}

func runDiagnose(cmd *cobra.Command, _ []string) error {
	snap, err := analyzer.Collect(flagGames)
	if err != nil {
		return fmt.Errorf("collection error: %w", err)
	}

	if len(snap.GameProcs) > 0 {
		names := make([]string, 0, len(snap.GameProcs))
		seen := map[string]bool{}
		for _, p := range snap.GameProcs {
			if !seen[p.Name] {
				names = append(names, p.Name)
				seen[p.Name] = true
			}
		}
		fmt.Fprintf(os.Stderr, "🎮  auto-detected game(s): %s\n", strings.Join(names, ", "))
	}

	findings := analyzer.Analyze(snap)

	var applied []fixer.Result
	if flagAutoFix {
		applied = fixer.ApplyAll(findings, flagSudo)
	}

	r := report.FullReport{
		GeneratedAt: time.Now(),
		Snapshot:    snap,
		Findings:    findings,
		Applied:     applied,
	}

	if err := writeReport(r); err != nil {
		return err
	}
	return runLLMEnhance(r, os.Stdout)
}

func runFix(cmd *cobra.Command, _ []string) error {
	flagAutoFix = true
	return runDiagnose(cmd, nil)
}

func runMonitor(cmd *cobra.Command, _ []string) error {
	gameDesc := "any game (auto-detect)"
	if len(flagGames) > 0 {
		gameDesc = strings.Join(flagGames, ", ")
	}
	fmt.Fprintf(os.Stderr, "🎮  gameperf monitor — watching for %s every %ds\n",
		gameDesc, flagInterval)

	ticker := time.NewTicker(time.Duration(flagInterval) * time.Second)
	defer ticker.Stop()

	for {
		snap, err := analyzer.Collect(flagGames)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection error: %v\n", err)
		} else {
			findings := analyzer.Analyze(snap)
			r := report.FullReport{
				GeneratedAt: time.Now(),
				Snapshot:    snap,
				Findings:    findings,
			}
			report.WriteConsole(os.Stdout, r)
		}
		<-ticker.C
	}
}

func runReport(cmd *cobra.Command, _ []string) error {
	snap, err := analyzer.Collect(flagGames)
	if err != nil {
		return fmt.Errorf("collection error: %w", err)
	}
	findings := analyzer.Analyze(snap)
	r := report.FullReport{
		GeneratedAt: time.Now(),
		Snapshot:    snap,
		Findings:    findings,
	}

	f, err := os.Create(flagOutput)
	if err != nil {
		return err
	}
	defer f.Close()

	switch flagFormat {
	case "json":
		err = report.WriteJSON(f, r)
	case "console":
		report.WriteConsole(f, r)
	default: // markdown
		report.WriteMarkdown(f, r)
	}
	if err != nil {
		return err
	}

	// Append LLM analysis to the report file when --llm is set.
	if flagLLM {
		if aiErr := runLLMEnhance(r, f); aiErr != nil {
			fmt.Fprintf(os.Stderr, "llm: %v\n", aiErr)
		}
	}

	fmt.Printf("Report written to %s\n", flagOutput)
	return nil
}

func writeReport(r report.FullReport) error {
	switch flagFormat {
	case "json":
		return report.WriteJSON(os.Stdout, r)
	case "markdown":
		report.WriteMarkdown(os.Stdout, r)
	default:
		report.WriteConsole(os.Stdout, r)
	}
	return nil
}

// runLLMEnhance loads the LLM provider, calls EnhanceReport, and writes the
// AI analysis to w. It is a no-op when --llm is not set.
func runLLMEnhance(r report.FullReport, w *os.File) error {
	if !flagLLM {
		return nil
	}

	cfg, err := llm.LoadConfig()
	if err != nil {
		return fmt.Errorf("load llm config: %w", err)
	}
	// CLI flags override config file.
	if flagLLMProvider != "" {
		cfg.Provider = flagLLMProvider
	}
	if flagLLMModel != "" {
		cfg.Model = flagLLMModel
	}

	provider, err := llm.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("create llm provider: %w", err)
	}

	fmt.Fprintf(os.Stderr, "🤖  Asking %s for analysis…\n", provider.Name())
	analysis, err := llm.EnhanceReport(context.Background(), provider, r)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## 🤖 AI Analysis")
	fmt.Fprintln(w, strings.TrimSpace(analysis))
	fmt.Fprintln(w)
	return nil
}
