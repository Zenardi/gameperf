package collector_test

import (
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

func TestParseVMMaxMapCount_Valid(t *testing.T) {
	v, err := collector.ParseVMMaxMapCount("65530\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 65530 {
		t.Errorf("ParseVMMaxMapCount = %d, want 65530", v)
	}
}

func TestParseVMMaxMapCount_LargeValue(t *testing.T) {
	v, err := collector.ParseVMMaxMapCount("2097152\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 2097152 {
		t.Errorf("ParseVMMaxMapCount = %d, want 2097152", v)
	}
}

func TestParseVMMaxMapCount_Invalid(t *testing.T) {
	_, err := collector.ParseVMMaxMapCount("not-a-number")
	if err == nil {
		t.Error("expected error for non-numeric input")
	}
}

func TestParseCPUGovernor_TrimWhitespace(t *testing.T) {
	if got := collector.ParseCPUGovernor("  powersave\n"); got != "powersave" {
		t.Errorf("ParseCPUGovernor = %q, want powersave", got)
	}
}

func TestParseCPUGovernor_Performance(t *testing.T) {
	if got := collector.ParseCPUGovernor("performance\n"); got != "performance" {
		t.Errorf("ParseCPUGovernor = %q, want performance", got)
	}
}

func TestCPUFreqInfo_ThrottlePercent_Throttled(t *testing.T) {
	f := collector.CPUFreqInfo{ID: 0, CurFreq: 2100000, MaxFreq: 3000000}
	// throttle = (1 - 2100000/3000000) * 100 = 30%
	got := f.ThrottlePercent()
	if got < 29.9 || got > 30.1 {
		t.Errorf("ThrottlePercent() = %.2f, want ~30", got)
	}
}

func TestCPUFreqInfo_ThrottlePercent_NoThrottle(t *testing.T) {
	f := collector.CPUFreqInfo{ID: 0, CurFreq: 3000000, MaxFreq: 3000000}
	if got := f.ThrottlePercent(); got != 0 {
		t.Errorf("ThrottlePercent() with cur==max = %f, want 0", got)
	}
}

func TestCPUFreqInfo_ThrottlePercent_ZeroMax(t *testing.T) {
	f := collector.CPUFreqInfo{ID: 0, CurFreq: 1000000, MaxFreq: 0}
	if got := f.ThrottlePercent(); got != 0 {
		t.Errorf("ThrottlePercent() with zero max = %f, want 0", got)
	}
}

func TestCPUFreqInfo_ThrottlePercent_AboveMax(t *testing.T) {
	// Turbo boost: cur > max should return 0 (not negative)
	f := collector.CPUFreqInfo{ID: 0, CurFreq: 3200000, MaxFreq: 3000000}
	if got := f.ThrottlePercent(); got != 0 {
		t.Errorf("ThrottlePercent() with turbo boost = %f, want 0", got)
	}
}
