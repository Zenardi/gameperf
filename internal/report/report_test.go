package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/report"
)

func sampleReport() report.FullReport {
	return report.FullReport{
		GeneratedAt: time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC),
		Findings: []analyzer.Finding{
			{
				ID:          "irq-ecore-217",
				Title:       "NVIDIA IRQ 217 is pinned to E-cores",
				Severity:    analyzer.SeverityCritical,
				Description: "IRQ 217 (nvidia) has most interrupts on E-core CPU2.",
				ManualFix:   "echo 0-7 | sudo tee /proc/irq/217/smp_affinity_list",
				AutoFixable: true,
				AutoFixCmd:  []string{"tee", "/proc/irq/217/smp_affinity_list"},
			},
			{
				ID:          "vram-pressure",
				Title:       "High VRAM usage",
				Severity:    analyzer.SeverityWarning,
				Description: "VRAM usage is at 87%.",
				InGameFix:   "Lower texture quality in Graphics Settings.",
				AutoFixable: false,
			},
		},
	}
}

// --- WriteConsole ---

func TestWriteConsole_ContainsSeverityLabels(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteConsole(&buf, r)
	out := buf.String()
	// WriteConsole shows finding titles — verify both findings appear
	if !strings.Contains(out, "NVIDIA IRQ 217") {
		t.Error("console output missing critical finding title")
	}
	if !strings.Contains(out, "High VRAM") {
		t.Error("console output missing warning finding title")
	}
}

func TestWriteConsole_ContainsFindingTitles(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteConsole(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "NVIDIA IRQ 217") {
		t.Error("console output missing IRQ finding title")
	}
	if !strings.Contains(out, "VRAM") {
		t.Error("console output missing VRAM finding title")
	}
}

func TestWriteConsole_ShowsFixInstructions(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteConsole(&buf, r)
	out := buf.String()
	// WriteConsole prints "(run `gameperf fix` to auto-fix)" for auto-fixable findings
	if !strings.Contains(out, "gameperf fix") {
		t.Error("console output should hint at gameperf fix for auto-fixable findings")
	}
}

func TestWriteConsole_EmptyReport_NoFindings(t *testing.T) {
	r := report.FullReport{GeneratedAt: time.Now()}
	var buf bytes.Buffer
	report.WriteConsole(&buf, r)
	out := buf.String()
	if len(out) == 0 {
		t.Error("console output should not be empty even for no-findings report")
	}
}

// --- WriteMarkdown ---

func TestWriteMarkdown_HasH1Header(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteMarkdown(&buf, r)
	out := buf.String()

	if !strings.HasPrefix(out, "# ") {
		t.Error("markdown output should start with an H1 header")
	}
}

func TestWriteMarkdown_ContainsFindingTitles(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteMarkdown(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "NVIDIA IRQ 217") {
		t.Error("markdown missing IRQ finding title")
	}
	if !strings.Contains(out, "High VRAM") {
		t.Error("markdown missing VRAM finding title")
	}
}

func TestWriteMarkdown_ContainsCodeBlock(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteMarkdown(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "```") {
		t.Error("markdown output should contain a fenced code block for fix commands")
	}
	if !strings.Contains(out, "smp_affinity_list") {
		t.Error("markdown output should contain the fix command inside a code block")
	}
}

func TestWriteMarkdown_ContainsInGameFix(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteMarkdown(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "texture quality") {
		t.Error("markdown output should include the in-game fix instruction")
	}
}

func TestWriteMarkdown_ContainsTimestamp(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteMarkdown(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "2025") {
		t.Error("markdown output should contain the collection year")
	}
}

// --- WriteJSON ---

func TestWriteJSON_ValidJSON(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteJSON(&buf, r)

	var decoded interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Errorf("WriteJSON produced invalid JSON: %v", err)
	}
}

func TestWriteJSON_ContainsExpectedFields(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteJSON(&buf, r)

	var decoded map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON decode error: %v", err)
	}

	if _, ok := decoded["findings"]; !ok {
		t.Error("JSON missing 'findings' key")
	}
	if _, ok := decoded["generated_at"]; !ok {
		t.Error("JSON missing 'generated_at' key")
	}
}

func TestWriteJSON_FindingsCount(t *testing.T) {
	r := sampleReport()
	var buf bytes.Buffer
	report.WriteJSON(&buf, r)

	var decoded struct {
		Findings []interface{} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON decode error: %v", err)
	}
	if len(decoded.Findings) != 2 {
		t.Errorf("JSON findings count = %d, want 2", len(decoded.Findings))
	}
}
