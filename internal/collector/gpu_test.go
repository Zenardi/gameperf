package collector_test

import (
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

func TestParseGPUOutput_AllFields(t *testing.T) {
	input := "99, 6, 39.22, 2812, 7683, 8151, 580.126.09"
	stat, err := collector.ParseGPUOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stat.UtilizationGPU != 99 {
		t.Errorf("UtilizationGPU = %d, want 99", stat.UtilizationGPU)
	}
	if stat.UtilizationMemory != 6 {
		t.Errorf("UtilizationMemory = %d, want 6", stat.UtilizationMemory)
	}
	if stat.PowerDraw != 39.22 {
		t.Errorf("PowerDraw = %f, want 39.22", stat.PowerDraw)
	}
	if stat.ClockGraphics != 2812 {
		t.Errorf("ClockGraphics = %d, want 2812", stat.ClockGraphics)
	}
	if stat.MemoryUsed != 7683 {
		t.Errorf("MemoryUsed = %d, want 7683", stat.MemoryUsed)
	}
	if stat.MemoryTotal != 8151 {
		t.Errorf("MemoryTotal = %d, want 8151", stat.MemoryTotal)
	}
	if stat.DriverVersion != "580.126.09" {
		t.Errorf("DriverVersion = %q, want 580.126.09", stat.DriverVersion)
	}
}

func TestParseGPUOutput_CommaSeparatedNoSpaces(t *testing.T) {
	input := "50,3,25.00,1500,4096,8192,550.00"
	stat, err := collector.ParseGPUOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stat.UtilizationGPU != 50 {
		t.Errorf("UtilizationGPU = %d, want 50", stat.UtilizationGPU)
	}
	if stat.MemoryTotal != 8192 {
		t.Errorf("MemoryTotal = %d, want 8192", stat.MemoryTotal)
	}
}

func TestParseGPUOutput_TooFewFields(t *testing.T) {
	input := "99, 6, 39.22"
	stat, err := collector.ParseGPUOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return zero-value GPUStat, not crash.
	if stat.UtilizationGPU != 0 {
		t.Errorf("expected zero-value GPUStat for short input")
	}
}

func TestGPUStat_MemoryUsedPercent(t *testing.T) {
	stat := collector.GPUStat{MemoryUsed: 7683, MemoryTotal: 8151}
	got := stat.MemoryUsedPercent()
	want := float64(7683) / float64(8151) * 100
	if got < want-0.01 || got > want+0.01 {
		t.Errorf("MemoryUsedPercent() = %.4f, want %.4f", got, want)
	}
}

func TestGPUStat_MemoryUsedPercent_ZeroTotal(t *testing.T) {
	stat := collector.GPUStat{MemoryUsed: 1000, MemoryTotal: 0}
	if got := stat.MemoryUsedPercent(); got != 0 {
		t.Errorf("MemoryUsedPercent() with zero total = %f, want 0", got)
	}
}
