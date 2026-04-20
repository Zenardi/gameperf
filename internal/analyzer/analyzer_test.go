package analyzer_test

import (
	"strings"
	"testing"

	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/collector"
)

// buildSnapshot creates a synthetic Snapshot for testing analyzer rules.
func buildSnapshot(opts ...func(*analyzer.Snapshot)) analyzer.Snapshot {
	snap := analyzer.Snapshot{
		CPUTopology: []collector.CPUTopology{
			{ID: 0, MaxFreq: 5100000}, // P-core
			{ID: 1, MaxFreq: 5000000}, // P-core
			{ID: 2, MaxFreq: 3000000}, // E-core
			{ID: 3, MaxFreq: 2900000}, // E-core
		},
	}
	for _, o := range opts {
		o(&snap)
	}
	return snap
}

func withNvidiaIRQOnECore() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		// IRQ 217 = nvidia, all interrupts on CPU2 (E-core)
		s.IRQs = []collector.IRQEntry{
			{
				Number: "217",
				Name:   "PCI-MSI nvidia",
				PerCPU: []int64{1000, 0, 3000000, 0},
				Total:  3001000,
			},
		}
		s.CPUCount = 4
	}
}

func withNvidiaIRQOnPCore() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.IRQs = []collector.IRQEntry{
			{
				Number: "217",
				Name:   "PCI-MSI nvidia",
				PerCPU: []int64{3000000, 0, 0, 0},
				Total:  3000000,
			},
		}
		s.CPUCount = 4
	}
}

func withVRAMPressure(usedMiB, totalMiB int64) func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.GPU = collector.GPUStat{
			MemoryUsed:  usedMiB,
			MemoryTotal: totalMiB,
		}
	}
}

func withGameProcess() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.GameProcs = []collector.ProcessInfo{
			{PID: 12345, Name: "ff7rebirth_.exe"},
		}
	}
}

// --- IRQ E-core pinning tests ---

func TestAnalyze_IRQOnECore_ProducesCriticalFinding(t *testing.T) {
	snap := buildSnapshot(withNvidiaIRQOnECore())
	findings := analyzer.Analyze(snap)

	var found *analyzer.Finding
	for i := range findings {
		if strings.HasPrefix(findings[i].ID, "irq-ecore-") {
			found = &findings[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected irq-ecore finding, got none")
	}
	if found.Severity != analyzer.SeverityCritical {
		t.Errorf("Severity = %s, want critical", found.Severity)
	}
	if !found.AutoFixable {
		t.Error("IRQ E-core finding should be AutoFixable")
	}
	if len(found.AutoFixCmd) == 0 {
		t.Error("AutoFixCmd should not be empty")
	}
	if found.ManualFix == "" {
		t.Error("ManualFix should not be empty")
	}
}

func TestAnalyze_IRQOnECore_FindingContainsIRQNumber(t *testing.T) {
	snap := buildSnapshot(withNvidiaIRQOnECore())
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if strings.HasPrefix(f.ID, "irq-ecore-") {
			if !strings.Contains(f.ID, "217") {
				t.Errorf("finding ID %q should contain IRQ number 217", f.ID)
			}
			if !strings.Contains(f.ManualFix, "217") {
				t.Errorf("ManualFix should reference IRQ 217")
			}
			return
		}
	}
	t.Fatal("irq-ecore finding not found")
}

func TestAnalyze_IRQOnPCore_NoFinding(t *testing.T) {
	snap := buildSnapshot(withNvidiaIRQOnPCore())
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if strings.HasPrefix(f.ID, "irq-ecore-") {
			t.Errorf("unexpected irq-ecore finding when IRQ is on P-core: %s", f.Title)
		}
	}
}

// --- VRAM pressure tests ---

func TestAnalyze_VRAMCritical_At94Percent(t *testing.T) {
	snap := buildSnapshot(withVRAMPressure(7658, 8151))
	findings := analyzer.Analyze(snap)

	var found *analyzer.Finding
	for i := range findings {
		if findings[i].ID == "vram-pressure" {
			found = &findings[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected vram-pressure finding at 94%")
	}
	if found.Severity != analyzer.SeverityCritical {
		t.Errorf("Severity = %s, want critical", found.Severity)
	}
	if found.AutoFixable {
		t.Error("VRAM pressure should not be AutoFixable (requires in-game action)")
	}
	if found.InGameFix == "" {
		t.Error("InGameFix should not be empty")
	}
}

func TestAnalyze_VRAMWarning_At87Percent(t *testing.T) {
	snap := buildSnapshot(withVRAMPressure(7091, 8151)) // ~87%
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if f.ID == "vram-pressure" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("at 87%% VRAM: Severity = %s, want warning", f.Severity)
			}
			return
		}
	}
	t.Fatal("expected vram-pressure finding at 87%")
}

func TestAnalyze_VRAM_NoFindingBelow85Percent(t *testing.T) {
	snap := buildSnapshot(withVRAMPressure(6000, 8151)) // ~73%
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if f.ID == "vram-pressure" {
			t.Errorf("unexpected vram-pressure finding at 73%%: %s", f.Title)
		}
	}
}

func TestAnalyze_VRAM_ZeroTotal_NoFinding(t *testing.T) {
	snap := buildSnapshot(withVRAMPressure(0, 0))
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if f.ID == "vram-pressure" {
			t.Error("vram-pressure finding with zero total VRAM (no GPU)")
		}
	}
}

// --- Game not running tests ---

func TestAnalyze_GameNotRunning_ProducesInfoFinding(t *testing.T) {
	snap := buildSnapshot() // no game process
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if f.ID == "game-not-running" {
			if f.Severity != analyzer.SeverityInfo {
				t.Errorf("Severity = %s, want info", f.Severity)
			}
			return
		}
	}
	t.Fatal("expected game-not-running finding")
}

func TestAnalyze_GameRunning_NoNotRunningFinding(t *testing.T) {
	snap := buildSnapshot(withGameProcess())
	findings := analyzer.Analyze(snap)

	for _, f := range findings {
		if f.ID == "game-not-running" {
			t.Error("unexpected game-not-running finding when game is running")
		}
	}
}

// --- Report helpers ---

func TestReport_HasCritical(t *testing.T) {
	r := analyzer.Report{
		Findings: []analyzer.Finding{
			{ID: "a", Severity: analyzer.SeverityInfo},
			{ID: "b", Severity: analyzer.SeverityCritical},
		},
	}
	if !r.HasCritical() {
		t.Error("HasCritical() = false, want true")
	}
}

func TestReport_HasCritical_False(t *testing.T) {
	r := analyzer.Report{
		Findings: []analyzer.Finding{
			{ID: "a", Severity: analyzer.SeverityInfo},
			{ID: "b", Severity: analyzer.SeverityWarning},
		},
	}
	if r.HasCritical() {
		t.Error("HasCritical() = true, want false")
	}
}

func TestReport_FindingByID(t *testing.T) {
	r := analyzer.Report{
		Findings: []analyzer.Finding{
			{ID: "foo", Title: "Foo finding"},
			{ID: "bar", Title: "Bar finding"},
		},
	}
	f := r.FindingByID("bar")
	if f == nil {
		t.Fatal("FindingByID(bar) returned nil")
	}
	if f.Title != "Bar finding" {
		t.Errorf("Title = %q, want 'Bar finding'", f.Title)
	}
	if r.FindingByID("missing") != nil {
		t.Error("FindingByID(missing) should return nil")
	}
}

// --- CPU governor tests ---

func withPowersaveGovernor() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.CPUGovernors = []collector.CPUGovernor{
			{ID: 0, Governor: "powersave"},
			{ID: 1, Governor: "powersave"},
			{ID: 2, Governor: "powersave"}, // E-core
			{ID: 3, Governor: "powersave"}, // E-core
		}
	}
}

func withPerformanceGovernor() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.CPUGovernors = []collector.CPUGovernor{
			{ID: 0, Governor: "performance"},
			{ID: 1, Governor: "performance"},
			{ID: 2, Governor: "performance"},
			{ID: 3, Governor: "performance"},
		}
	}
}

func withLowVMMaxMapCount() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.VMMaxMapCount = 65530 }
}

func withHighVMMaxMapCount() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.VMMaxMapCount = 2097152 }
}

func withTHPAlways() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.THPMode = "always" }
}

func withTHPMadvise() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.THPMode = "madvise" }
}

func withHighSwap() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.MemInfo = collector.MemInfo{
			MemTotal: 32000000, MemAvailable: 10000000,
			SwapTotal: 8000000, SwapFree: 1000000, // 87.5% used
		}
	}
}

func withNoSwap() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.MemInfo = collector.MemInfo{
			MemTotal: 32000000, MemAvailable: 20000000,
			SwapTotal: 0, SwapFree: 0,
		}
	}
}

func withLowRAM() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.MemInfo = collector.MemInfo{
			MemTotal: 16000000, MemAvailable: 1024000, // ~1 GiB available
		}
	}
}

func withHighRAM() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.MemInfo = collector.MemInfo{
			MemTotal: 32000000, MemAvailable: 20000000,
		}
	}
}

func withThrottledCPU() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		// CPU0 and CPU1 (P-cores) throttled to 50% of max
		s.CPUFreqs = []collector.CPUFreqInfo{
			{ID: 0, CurFreq: 1500000, MaxFreq: 5100000},
			{ID: 1, CurFreq: 1500000, MaxFreq: 5000000},
			{ID: 2, CurFreq: 3000000, MaxFreq: 3000000},
			{ID: 3, CurFreq: 2900000, MaxFreq: 2900000},
		}
	}
}

func withNormalCPUFreq() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) {
		s.CPUFreqs = []collector.CPUFreqInfo{
			{ID: 0, CurFreq: 5050000, MaxFreq: 5100000},
			{ID: 1, CurFreq: 4900000, MaxFreq: 5000000},
		}
	}
}

func withIRQBalanceRunning() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.IRQBalanceRunning = true }
}

func withIRQBalanceMissing() func(*analyzer.Snapshot) {
	return func(s *analyzer.Snapshot) { s.IRQBalanceRunning = false }
}

func TestAnalyze_CPUGovernor_Powersave_Critical(t *testing.T) {
	snap := buildSnapshot(withPowersaveGovernor())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "cpu-governor-powersave" {
			if f.Severity != analyzer.SeverityCritical {
				t.Errorf("Severity = %s, want critical", f.Severity)
			}
			if !f.AutoFixable {
				t.Error("cpu-governor-powersave should be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected cpu-governor-powersave finding")
}

func TestAnalyze_CPUGovernor_Performance_NoFinding(t *testing.T) {
	snap := buildSnapshot(withPerformanceGovernor())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "cpu-governor-powersave" {
			t.Error("unexpected cpu-governor-powersave finding with performance governor")
		}
	}
}

func TestAnalyze_CPUGovernor_NoGovernors_NoFinding(t *testing.T) {
	snap := buildSnapshot() // no CPUGovernors
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "cpu-governor-powersave" {
			t.Error("unexpected cpu-governor finding with no governor data")
		}
	}
}

func TestAnalyze_VMMaxMapCount_Low_Warning(t *testing.T) {
	snap := buildSnapshot(withLowVMMaxMapCount())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "vm-max-map-count" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("Severity = %s, want warning", f.Severity)
			}
			if !f.AutoFixable {
				t.Error("vm-max-map-count should be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected vm-max-map-count finding")
}

func TestAnalyze_VMMaxMapCount_High_NoFinding(t *testing.T) {
	snap := buildSnapshot(withHighVMMaxMapCount())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "vm-max-map-count" {
			t.Error("unexpected vm-max-map-count finding when value is high enough")
		}
	}
}

func TestAnalyze_VMMaxMapCount_Zero_NoFinding(t *testing.T) {
	snap := buildSnapshot() // VMMaxMapCount = 0 (not collected)
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "vm-max-map-count" {
			t.Error("unexpected vm-max-map-count finding when value is 0 (not collected)")
		}
	}
}

func TestAnalyze_THP_Always_Warning(t *testing.T) {
	snap := buildSnapshot(withTHPAlways())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "thp-always" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("Severity = %s, want warning", f.Severity)
			}
			if !f.AutoFixable {
				t.Error("thp-always should be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected thp-always finding")
}

func TestAnalyze_THP_Madvise_NoFinding(t *testing.T) {
	snap := buildSnapshot(withTHPMadvise())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "thp-always" {
			t.Error("unexpected thp-always finding when THP is madvise")
		}
	}
}

func TestAnalyze_SwapPressure_Critical_At87Percent(t *testing.T) {
	snap := buildSnapshot(withHighSwap())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "swap-pressure" {
			if f.Severity != analyzer.SeverityCritical {
				t.Errorf("at 87%% swap: Severity = %s, want critical", f.Severity)
			}
			if f.AutoFixable {
				t.Error("swap-pressure should not be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected swap-pressure finding at 87%")
}

func TestAnalyze_SwapPressure_NoSwap_NoFinding(t *testing.T) {
	snap := buildSnapshot(withNoSwap())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "swap-pressure" {
			t.Error("unexpected swap-pressure finding when no swap configured")
		}
	}
}

func TestAnalyze_RAMPressure_LowRAM_Warning(t *testing.T) {
	snap := buildSnapshot(withLowRAM())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "ram-pressure" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("Severity = %s, want warning", f.Severity)
			}
			return
		}
	}
	t.Fatal("expected ram-pressure finding")
}

func TestAnalyze_RAMPressure_HighRAM_NoFinding(t *testing.T) {
	snap := buildSnapshot(withHighRAM())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "ram-pressure" {
			t.Error("unexpected ram-pressure finding when RAM is plentiful")
		}
	}
}

func TestAnalyze_CPUThrottling_Throttled_Warning(t *testing.T) {
	snap := buildSnapshot(withThrottledCPU())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "cpu-throttling" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("Severity = %s, want warning", f.Severity)
			}
			if f.AutoFixable {
				t.Error("cpu-throttling should not be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected cpu-throttling finding")
}

func TestAnalyze_CPUThrottling_NormalFreq_NoFinding(t *testing.T) {
	snap := buildSnapshot(withNormalCPUFreq())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "cpu-throttling" {
			t.Error("unexpected cpu-throttling finding at normal frequency")
		}
	}
}

func TestAnalyze_IRQBalanceMissing_Warning(t *testing.T) {
	snap := buildSnapshot(withIRQBalanceMissing())
	findings := analyzer.Analyze(snap)
	for _, f := range findings {
		if f.ID == "irqbalance-missing" {
			if f.Severity != analyzer.SeverityWarning {
				t.Errorf("Severity = %s, want warning", f.Severity)
			}
			if !f.AutoFixable {
				t.Error("irqbalance-missing should be AutoFixable")
			}
			return
		}
	}
	t.Fatal("expected irqbalance-missing finding")
}

func TestAnalyze_IRQBalanceRunning_NoFinding(t *testing.T) {
	snap := buildSnapshot(withIRQBalanceRunning())
	for _, f := range analyzer.Analyze(snap) {
		if f.ID == "irqbalance-missing" {
			t.Error("unexpected irqbalance-missing finding when irqbalance is running")
		}
	}
}
