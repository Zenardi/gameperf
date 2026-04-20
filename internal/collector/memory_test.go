package collector_test

import (
	"strings"
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

const procMeminfo = `MemTotal:       32768000 kB
MemFree:         4096000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          6000000 kB
SwapCached:            0 kB
SwapTotal:       8192000 kB
SwapFree:        4096000 kB
`

func TestParseMemInfo_Fields(t *testing.T) {
	info, err := collector.ParseMemInfo(strings.NewReader(procMeminfo))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.MemTotal != 32768000 {
		t.Errorf("MemTotal = %d, want 32768000", info.MemTotal)
	}
	if info.MemAvailable != 8192000 {
		t.Errorf("MemAvailable = %d, want 8192000", info.MemAvailable)
	}
	if info.SwapTotal != 8192000 {
		t.Errorf("SwapTotal = %d, want 8192000", info.SwapTotal)
	}
	if info.SwapFree != 4096000 {
		t.Errorf("SwapFree = %d, want 4096000", info.SwapFree)
	}
}

func TestParseMemInfo_SwapUsedPercent(t *testing.T) {
	info, _ := collector.ParseMemInfo(strings.NewReader(procMeminfo))
	// swap used = 8192000 - 4096000 = 4096000 / 8192000 = 50%
	got := info.SwapUsedPercent()
	if got < 49.9 || got > 50.1 {
		t.Errorf("SwapUsedPercent() = %.2f, want ~50", got)
	}
}

func TestParseMemInfo_AvailablePercent(t *testing.T) {
	info, _ := collector.ParseMemInfo(strings.NewReader(procMeminfo))
	// 8192000 / 32768000 = 25%
	got := info.AvailablePercent()
	if got < 24.9 || got > 25.1 {
		t.Errorf("AvailablePercent() = %.2f, want ~25", got)
	}
}

func TestParseMemInfo_AvailableMiB(t *testing.T) {
	info, _ := collector.ParseMemInfo(strings.NewReader(procMeminfo))
	// 8192000 kB / 1024 = 8000 MiB
	if got := info.AvailableMiB(); got != 8000 {
		t.Errorf("AvailableMiB() = %d, want 8000", got)
	}
}

func TestParseMemInfo_ZeroSwapTotal_NoDiv(t *testing.T) {
	info := collector.MemInfo{SwapTotal: 0, SwapFree: 0}
	if got := info.SwapUsedPercent(); got != 0 {
		t.Errorf("SwapUsedPercent() with zero total = %f, want 0", got)
	}
}

func TestParseMemInfo_ZeroMemTotal_NoDiv(t *testing.T) {
	info := collector.MemInfo{MemTotal: 0, MemAvailable: 0}
	if got := info.AvailablePercent(); got != 100 {
		t.Errorf("AvailablePercent() with zero total = %f, want 100", got)
	}
}

func TestParseTHPMode_MadviseSelected(t *testing.T) {
	if got := collector.ParseTHPMode("always [madvise] never"); got != "madvise" {
		t.Errorf("ParseTHPMode = %q, want madvise", got)
	}
}

func TestParseTHPMode_AlwaysSelected(t *testing.T) {
	if got := collector.ParseTHPMode("[always] madvise never"); got != "always" {
		t.Errorf("ParseTHPMode = %q, want always", got)
	}
}

func TestParseTHPMode_NeverSelected(t *testing.T) {
	if got := collector.ParseTHPMode("always madvise [never]"); got != "never" {
		t.Errorf("ParseTHPMode = %q, want never", got)
	}
}

func TestParseTHPMode_NoBrackets(t *testing.T) {
	// Fallback: return trimmed content
	if got := collector.ParseTHPMode("  always  "); got != "always" {
		t.Errorf("ParseTHPMode no-brackets = %q, want always", got)
	}
}
