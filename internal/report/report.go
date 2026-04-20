package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/fixer"
)

// FullReport bundles everything produced in one analysis run.
type FullReport struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Snapshot    analyzer.Snapshot  `json:"snapshot"`
	Findings    []analyzer.Finding `json:"findings"`
	Applied     []fixer.Result     `json:"applied"`
}

// WriteMarkdown writes a human-readable Markdown report to w.
func WriteMarkdown(w io.Writer, r FullReport) {
	fmt.Fprintf(w, "# gameperf Report\n\n")
	fmt.Fprintf(w, "_Generated: %s_\n\n", r.GeneratedAt.Format(time.RFC1123))

	// System snapshot
	fmt.Fprintf(w, "## System Snapshot\n\n")
	g := r.Snapshot.GPU
	if g.MemoryTotal > 0 {
		fmt.Fprintf(w, "| Metric | Value |\n|---|---|\n")
		fmt.Fprintf(w, "| GPU Utilization | %d%% |\n", g.UtilizationGPU)
		fmt.Fprintf(w, "| GPU Memory | %d / %d MiB (%.0f%%) |\n", g.MemoryUsed, g.MemoryTotal, g.MemoryUsedPercent())
		fmt.Fprintf(w, "| GPU Power | %.1f W |\n", g.PowerDraw)
		fmt.Fprintf(w, "| GPU Clock | %d MHz |\n", g.ClockGraphics)
		fmt.Fprintf(w, "| Driver | %s |\n", g.DriverVersion)
		fmt.Fprintf(w, "\n")
	}

	// Memory
	mem := r.Snapshot.MemInfo
	if mem.MemTotal > 0 {
		fmt.Fprintf(w, "| RAM Available | %d MiB / %d MiB (%.0f%%) |\n",
			mem.AvailableMiB(), mem.MemTotal/1024, mem.AvailablePercent())
		if mem.SwapTotal > 0 {
			fmt.Fprintf(w, "| Swap Usage | %.0f%% (%d / %d MiB) |\n",
				mem.SwapUsedPercent(), (mem.SwapTotal-mem.SwapFree)/1024, mem.SwapTotal/1024)
		}
	}
	if r.Snapshot.VMMaxMapCount > 0 {
		fmt.Fprintf(w, "| vm.max_map_count | %d |\n", r.Snapshot.VMMaxMapCount)
	}
	if r.Snapshot.THPMode != "" {
		fmt.Fprintf(w, "| Transparent HugePages | %s |\n", r.Snapshot.THPMode)
	}

	if len(r.Snapshot.GameProcs) > 0 {
		fmt.Fprintf(w, "**Game processes detected:**\n\n")
		for _, p := range r.Snapshot.GameProcs {
			fmt.Fprintf(w, "- PID %d: `%s`\n", p.PID, p.Name)
		}
		fmt.Fprintf(w, "\n")
	}

	// Findings
	fmt.Fprintf(w, "## Findings (%d)\n\n", len(r.Findings))
	if len(r.Findings) == 0 {
		fmt.Fprintf(w, "_No issues detected. System looks healthy._\n\n")
	}

	for _, f := range r.Findings {
		icon := severityIcon(f.Severity)
		fmt.Fprintf(w, "### %s %s `[%s]`\n\n", icon, f.Title, f.ID)
		fmt.Fprintf(w, "**Severity:** %s\n\n", strings.ToUpper(string(f.Severity)))
		fmt.Fprintf(w, "%s\n\n", f.Description)

		if f.Evidence != "" {
			fmt.Fprintf(w, "**Evidence:**\n```\n%s\n```\n\n", f.Evidence)
		}

		if f.AutoFixable {
			fmt.Fprintf(w, "✅ **Auto-fixable** — run `gameperf fix` to apply automatically.\n\n")
		}
		if f.ManualFix != "" {
			fmt.Fprintf(w, "**Manual fix:**\n\n```\n%s\n```\n\n", f.ManualFix)
		}
		if f.InGameFix != "" {
			fmt.Fprintf(w, "**In-game fix:**\n\n> %s\n\n", f.InGameFix)
		}
		fmt.Fprintf(w, "---\n\n")
	}

	// Applied fixes
	if len(r.Applied) > 0 {
		fmt.Fprintf(w, "## Applied Fixes\n\n")
		for _, a := range r.Applied {
			if a.Applied {
				fmt.Fprintf(w, "- ✅ `%s` — applied successfully\n", a.FindingID)
			} else {
				fmt.Fprintf(w, "- ❌ `%s` — failed: %v\n", a.FindingID, a.Err)
			}
			if a.Output != "" {
				fmt.Fprintf(w, "  ```\n  %s\n  ```\n", a.Output)
			}
		}
		fmt.Fprintf(w, "\n")
	}
}

// WriteJSON writes a machine-readable JSON report to w.
func WriteJSON(w io.Writer, r FullReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteConsole writes a compact coloured summary to w (terminal output).
func WriteConsole(w io.Writer, r FullReport) {
	fmt.Fprintf(w, "\n🎮  gameperf — %s\n\n", r.GeneratedAt.Format("15:04:05"))

	g := r.Snapshot.GPU
	if g.MemoryTotal > 0 {
		fmt.Fprintf(w, "  GPU  %d%%  VRAM %d/%d MiB (%.0f%%)  %s  %.1fW\n\n",
			g.UtilizationGPU, g.MemoryUsed, g.MemoryTotal, g.MemoryUsedPercent(),
			g.DriverVersion, g.PowerDraw)
	}

	if len(r.Findings) == 0 {
		fmt.Fprintf(w, "  ✅  No issues found.\n\n")
		return
	}

	for _, f := range r.Findings {
		icon := severityIcon(f.Severity)
		fix := ""
		if f.AutoFixable {
			fix = " (run `gameperf fix` to auto-fix)"
		}
		fmt.Fprintf(w, "  %s  %s%s\n", icon, f.Title, fix)
		fmt.Fprintf(w, "     %s\n\n", f.Description)
	}
}

func severityIcon(s analyzer.Severity) string {
	switch s {
	case analyzer.SeverityCritical:
		return "🔴"
	case analyzer.SeverityWarning:
		return "🟡"
	default:
		return "🔵"
	}
}

func codeBlock(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		out = append(out, "    "+l)
	}
	return strings.Join(out, "\n")
}
