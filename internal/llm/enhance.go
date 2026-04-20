package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenardi/gameperf/internal/report"
)

const systemContext = `You are a Linux gaming performance expert. You receive a diagnostic report from gameperf and provide concise, actionable analysis. Focus on root causes and their interaction — do not just restate the findings. Prioritize by impact. Keep your response under 400 words.`

// BuildPrompt constructs the full prompt sent to the LLM from a FullReport.
func BuildPrompt(r report.FullReport) string {
	var sb strings.Builder

	sb.WriteString(systemContext)
	sb.WriteString("\n\n---\n\n")

	// Snapshot summary
	sb.WriteString("## System Snapshot\n")
	sb.WriteString(fmt.Sprintf("- GPU VRAM: %.0f / %.0f MiB (%.1f%%)\n",
		float64(r.Snapshot.GPU.MemoryUsed),
		float64(r.Snapshot.GPU.MemoryTotal),
		r.Snapshot.GPU.MemoryUsedPercent()))
	sb.WriteString(fmt.Sprintf("- RAM Available: %.1f%%\n", r.Snapshot.MemInfo.AvailablePercent()))
	sb.WriteString(fmt.Sprintf("- Swap Used: %.1f%%\n", r.Snapshot.MemInfo.SwapUsedPercent()))
	sb.WriteString(fmt.Sprintf("- vm.max_map_count: %d\n", r.Snapshot.VMMaxMapCount))
	sb.WriteString(fmt.Sprintf("- THP Mode: %s\n", r.Snapshot.THPMode))
	sb.WriteString(fmt.Sprintf("- Game process detected: %v\n", len(r.Snapshot.GameProcs) > 0))

	// Findings
	sb.WriteString("\n## Diagnostic Findings\n")
	if len(r.Findings) == 0 {
		sb.WriteString("No issues detected.\n")
	} else {
		for _, f := range r.Findings {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n",
				strings.ToUpper(string(f.Severity)), f.ID, f.Title))
			if f.Evidence != "" {
				sb.WriteString(fmt.Sprintf("  Evidence: %s\n", f.Evidence))
			}
		}
	}

	sb.WriteString("\n---\n\nProvide your expert analysis and prioritized recommendations:")
	return sb.String()
}

// EnhanceReport sends the full report to the LLM provider and returns the
// model's analysis as a plain string. The caller decides how to format/display it.
func EnhanceReport(ctx context.Context, p Provider, r report.FullReport) (string, error) {
	prompt := BuildPrompt(r)
	response, err := p.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("llm %s: %w", p.Name(), err)
	}
	return response, nil
}
