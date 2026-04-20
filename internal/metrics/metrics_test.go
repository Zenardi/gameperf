package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/collector"
	"github.com/zenardi/gameperf/internal/metrics"
)

// buildSnap is a helper to create a minimal Snapshot for testing.
func buildSnap(opts ...func(*analyzer.Snapshot)) analyzer.Snapshot {
	snap := analyzer.Snapshot{
		CPUStats: []collector.CPUStat{
			{ID: 0, User: 100, System: 50, Idle: 850},
			{ID: 1, User: 200, System: 80, Idle: 720},
		},
		CPUGovernors: []collector.CPUGovernor{
			{ID: 0, Governor: "performance"},
			{ID: 1, Governor: "powersave"},
		},
		CPUFreqs: []collector.CPUFreqInfo{
			{ID: 0, CurFreq: 3_000_000, MaxFreq: 4_000_000},
			{ID: 1, CurFreq: 4_000_000, MaxFreq: 4_000_000},
		},
		GPU: collector.GPUStat{
			MemoryUsed:  4096,
			MemoryTotal: 8192,
		},
		MemInfo: collector.MemInfo{
			MemTotal:     16 * 1024 * 1024, // 16 GiB in kB
			MemAvailable: 8 * 1024 * 1024,
			SwapTotal:    4 * 1024 * 1024,
			SwapFree:     1 * 1024 * 1024,
		},
		VMMaxMapCount: 65530,
	}
	for _, o := range opts {
		o(&snap)
	}
	return snap
}

func advanceCPU(snap *analyzer.Snapshot, deltaUser, deltaIdle int64) {
	for i := range snap.CPUStats {
		snap.CPUStats[i].User += deltaUser
		snap.CPUStats[i].Idle += deltaIdle
	}
}

// TestNew_DoesNotPanic ensures New() registers all metrics without panicking.
func TestNew_DoesNotPanic(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

// TestUpdateFromSnapshot_CPUUsage verifies cpu_usage_percent is computed via delta.
func TestUpdateFromSnapshot_CPUUsage(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap()

	// First call — establishes baseline, no delta yet.
	m.UpdateFromSnapshot(snap, nil)

	// Second call — advance CPU 0 by +200 user, +800 idle (total +1000, 20% usage).
	advanceCPU(&snap, 200, 800)
	m.UpdateFromSnapshot(snap, nil)

	got := testutil.ToFloat64(m.CPUUsage.WithLabelValues("0"))
	// usage = 100 - idle% = 100 - (800/1000*100) = 20
	if got < 19.9 || got > 20.1 {
		t.Errorf("cpu_usage_percent{core=0} = %.2f, want ~20.0", got)
	}
}

// TestUpdateFromSnapshot_CPUUsage_FirstCallIsZero ensures no metrics are emitted
// before the first delta is available.
func TestUpdateFromSnapshot_CPUUsage_FirstCallIsZero(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap()
	m.UpdateFromSnapshot(snap, nil)

	// Before a second call there should be no cpu_usage_percent metrics at all.
	count, err := testutil.GatherAndCount(m.Registry, "gameperf_cpu_usage_percent")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 cpu_usage_percent metrics after first call, got %d", count)
	}
}

// TestUpdateFromSnapshot_CPUGovernor verifies cpu_governor_info labels are set.
func TestUpdateFromSnapshot_CPUGovernor(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap()
	m.UpdateFromSnapshot(snap, nil)

	got := testutil.ToFloat64(m.CPUGovernor.WithLabelValues("0", "performance"))
	if got != 1 {
		t.Errorf("cpu_governor_info{core=0, governor=performance} = %.0f, want 1", got)
	}
	got = testutil.ToFloat64(m.CPUGovernor.WithLabelValues("1", "powersave"))
	if got != 1 {
		t.Errorf("cpu_governor_info{core=1, governor=powersave} = %.0f, want 1", got)
	}
}

// TestUpdateFromSnapshot_CPUGovernor_Reset ensures stale governor labels are cleared.
func TestUpdateFromSnapshot_CPUGovernor_Reset(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap()
	m.UpdateFromSnapshot(snap, nil)

	// Switch core 0 to powersave.
	snap.CPUGovernors[0].Governor = "powersave"
	m.UpdateFromSnapshot(snap, nil)

	count, err := testutil.GatherAndCount(m.Registry, "gameperf_cpu_governor_info")
	if err != nil {
		t.Fatal(err)
	}
	// 2 cores × 1 governor each = 2 label sets (no stale "performance" label).
	if count != 2 {
		t.Errorf("expected 2 cpu_governor_info series after reset, got %d", count)
	}
}

// TestUpdateFromSnapshot_CPUThrottle verifies cpu_throttle_percent.
func TestUpdateFromSnapshot_CPUThrottle(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap() // core 0: 3GHz/4GHz = 25% throttle; core 1: 4/4 = 0%
	m.UpdateFromSnapshot(snap, nil)

	core0 := testutil.ToFloat64(m.CPUThrottle.WithLabelValues("0"))
	if core0 < 24.9 || core0 > 25.1 {
		t.Errorf("cpu_throttle_percent{core=0} = %.2f, want ~25.0", core0)
	}
	core1 := testutil.ToFloat64(m.CPUThrottle.WithLabelValues("1"))
	if core1 != 0 {
		t.Errorf("cpu_throttle_percent{core=1} = %.2f, want 0.0", core1)
	}
}

// TestUpdateFromSnapshot_GPUVRAM verifies GPU VRAM metrics.
func TestUpdateFromSnapshot_GPUVRAM(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap() // MemoryUsed=4096, MemoryTotal=8192
	m.UpdateFromSnapshot(snap, nil)

	used := testutil.ToFloat64(m.GPUVRAMUsed)
	if used != 4096 {
		t.Errorf("gpu_vram_used_mib = %.0f, want 4096", used)
	}
	total := testutil.ToFloat64(m.GPUVRAMTotal)
	if total != 8192 {
		t.Errorf("gpu_vram_total_mib = %.0f, want 8192", total)
	}
	pct := testutil.ToFloat64(m.GPUVRAMPct)
	if pct < 49.9 || pct > 50.1 {
		t.Errorf("gpu_vram_used_percent = %.2f, want ~50.0", pct)
	}
}

// TestUpdateFromSnapshot_Memory verifies RAM and swap metrics.
func TestUpdateFromSnapshot_Memory(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	// MemAvailable=8GiB of 16GiB → 50% available; SwapFree=1GiB of 4GiB → 75% used.
	snap := buildSnap()
	m.UpdateFromSnapshot(snap, nil)

	ramPct := testutil.ToFloat64(m.RAMAvailable)
	if ramPct < 49.9 || ramPct > 50.1 {
		t.Errorf("ram_available_percent = %.2f, want ~50.0", ramPct)
	}
	swapPct := testutil.ToFloat64(m.SwapUsed)
	if swapPct < 74.9 || swapPct > 75.1 {
		t.Errorf("swap_used_percent = %.2f, want ~75.0", swapPct)
	}
}

// TestUpdateFromSnapshot_VMMaxMapCount verifies vm_max_map_count metric.
func TestUpdateFromSnapshot_VMMaxMapCount(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	snap := buildSnap()
	m.UpdateFromSnapshot(snap, nil)

	got := testutil.ToFloat64(m.VMMaxMapCount)
	if got != 65530 {
		t.Errorf("vm_max_map_count = %.0f, want 65530", got)
	}
}

// TestUpdateFromSnapshot_Findings verifies finding_active labels and values.
func TestUpdateFromSnapshot_Findings(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	findings := []analyzer.Finding{
		{ID: "cpu_governor", Severity: analyzer.SeverityCritical},
		{ID: "vm_max_map_count", Severity: analyzer.SeverityWarning},
	}
	m.UpdateFromSnapshot(buildSnap(), findings)

	gov := testutil.ToFloat64(m.FindingActive.WithLabelValues("cpu_governor", "critical"))
	if gov != 1 {
		t.Errorf("finding_active{id=cpu_governor} = %.0f, want 1", gov)
	}
	mmc := testutil.ToFloat64(m.FindingActive.WithLabelValues("vm_max_map_count", "warning"))
	if mmc != 1 {
		t.Errorf("finding_active{id=vm_max_map_count} = %.0f, want 1", mmc)
	}
}

// TestUpdateFromSnapshot_Findings_Reset ensures stale findings are cleared.
func TestUpdateFromSnapshot_Findings_Reset(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	findings := []analyzer.Finding{
		{ID: "cpu_governor", Severity: analyzer.SeverityCritical},
	}
	m.UpdateFromSnapshot(buildSnap(), findings)

	// Second update with no findings — cpu_governor should be gone.
	m.UpdateFromSnapshot(buildSnap(), nil)

	count, err := testutil.GatherAndCount(m.Registry, "gameperf_finding_active")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 finding_active series after reset, got %d", count)
	}
}

// TestHandler_Returns200 verifies the /metrics HTTP handler responds correctly.
func TestHandler_Returns200(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	m.UpdateFromSnapshot(buildSnap(), nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handler returned %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// TestHandler_ContainsMetricNames verifies the /metrics body contains known metric names.
func TestHandler_ContainsMetricNames(t *testing.T) {
	t.Parallel()
	m := metrics.New()
	m.UpdateFromSnapshot(buildSnap(), []analyzer.Finding{
		{ID: "test_finding", Severity: analyzer.SeverityInfo},
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)

	body := w.Body.String()
	for _, name := range []string{
		"gameperf_cpu_governor_info",
		"gameperf_cpu_throttle_percent",
		"gameperf_gpu_vram_used_mib",
		"gameperf_gpu_vram_total_mib",
		"gameperf_gpu_vram_used_percent",
		"gameperf_ram_available_percent",
		"gameperf_swap_used_percent",
		"gameperf_vm_max_map_count",
		"gameperf_finding_active",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics body missing %q", name)
		}
	}
}
