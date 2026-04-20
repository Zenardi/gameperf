package collector_test

import (
	"strings"
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

// Minimal /proc/stat with 4 CPUs and an aggregate cpu line.
const procStat = `cpu  100 20 50 800 10 5 3 0 0 0
cpu0 30  5  15 200 3  1 1 0 0 0
cpu1 25  5  12 200 3  2 1 0 0 0
cpu2 25  5  13 200 2  1 1 0 0 0
cpu3 20  5  10 200 2  1 0 0 0 0
intr 123456
`

func TestParseCPUStats_Count(t *testing.T) {
	stats, err := collector.ParseCPUStats(strings.NewReader(procStat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 4 {
		t.Errorf("len(stats) = %d, want 4", len(stats))
	}
}

func TestParseCPUStats_Fields(t *testing.T) {
	stats, err := collector.ParseCPUStats(strings.NewReader(procStat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := stats[0]
	if s.User != 30 {
		t.Errorf("CPU0 User = %d, want 30", s.User)
	}
	if s.System != 15 {
		t.Errorf("CPU0 System = %d, want 15", s.System)
	}
	if s.Idle != 200 {
		t.Errorf("CPU0 Idle = %d, want 200", s.Idle)
	}
}

func TestParseCPUStats_IDsAreSequential(t *testing.T) {
	stats, err := collector.ParseCPUStats(strings.NewReader(procStat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, s := range stats {
		if s.ID != i {
			t.Errorf("stats[%d].ID = %d, want %d", i, s.ID, i)
		}
	}
}

func TestCPUStat_SysPercent(t *testing.T) {
	prev := collector.CPUStat{System: 10, IRQ: 2, SoftIRQ: 1, Idle: 500, User: 100}
	curr := collector.CPUStat{System: 20, IRQ: 4, SoftIRQ: 2, Idle: 600, User: 200}
	// sys diff = (20+4+2)-(10+2+1) = 26-13 = 13
	// total diff = (20+0+4+2+600+200) - (10+0+2+1+500+100) = 826 - 613 = 213
	got := curr.SysPercent(prev)
	want := float64(13) / float64(213) * 100
	if got < want-0.01 || got > want+0.01 {
		t.Errorf("SysPercent() = %.4f, want %.4f", got, want)
	}
}

func TestCPUStat_SysPercent_ZeroDiff(t *testing.T) {
	s := collector.CPUStat{System: 10, Idle: 100}
	if got := s.SysPercent(s); got != 0 {
		t.Errorf("SysPercent with identical snapshots = %f, want 0", got)
	}
}

func TestCPUStat_IdlePercent(t *testing.T) {
	prev := collector.CPUStat{Idle: 500, User: 100, System: 10}
	curr := collector.CPUStat{Idle: 600, User: 200, System: 20}
	// idle diff = 100, total diff = 210
	got := curr.IdlePercent(prev)
	want := float64(100) / float64(210) * 100
	if got < want-0.01 || got > want+0.01 {
		t.Errorf("IdlePercent() = %.4f, want %.4f", got, want)
	}
}

// Minimal /proc/cpuinfo with 4 CPUs: 2 P-cores (5000 MHz) + 2 E-cores (3000 MHz).
const procCPUInfo = `processor	: 0
cpu MHz		: 5000.000

processor	: 1
cpu MHz		: 5100.000

processor	: 2
cpu MHz		: 3000.000

processor	: 3
cpu MHz		: 2900.000
`

func TestParseCPUTopology_Count(t *testing.T) {
	topos, err := collector.ParseCPUTopology(strings.NewReader(procCPUInfo))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topos) != 4 {
		t.Errorf("len(topos) = %d, want 4", len(topos))
	}
}

func TestParseCPUTopology_Frequencies(t *testing.T) {
	topos, err := collector.ParseCPUTopology(strings.NewReader(procCPUInfo))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// CPU0 at 5000 MHz → 5000000 kHz
	if topos[0].MaxFreq != 5000000 {
		t.Errorf("CPU0 MaxFreq = %d, want 5000000", topos[0].MaxFreq)
	}
}

func TestPCoreIDs_ReturnsFastCores(t *testing.T) {
	topos, _ := collector.ParseCPUTopology(strings.NewReader(procCPUInfo))
	pCores := collector.PCoreIDs(topos)
	// Only CPU0 (5000 MHz) and CPU1 (5100 MHz) are within 10% of max (5100 MHz)
	// CPU2 (3000 MHz) and CPU3 (2900 MHz) are E-cores
	if len(pCores) != 2 {
		t.Errorf("PCoreIDs() = %v (len %d), want 2 P-cores", pCores, len(pCores))
	}
	pSet := map[int]bool{}
	for _, id := range pCores {
		pSet[id] = true
	}
	if !pSet[0] || !pSet[1] {
		t.Errorf("PCoreIDs() = %v, expected CPU 0 and 1", pCores)
	}
	if pSet[2] || pSet[3] {
		t.Errorf("PCoreIDs() = %v, E-cores 2 and 3 should be excluded", pCores)
	}
}

func TestPCoreIDs_EmptyInput(t *testing.T) {
	if ids := collector.PCoreIDs(nil); ids != nil {
		t.Errorf("PCoreIDs(nil) = %v, want nil", ids)
	}
}

func TestPCoreIDs_AllSameFrequency(t *testing.T) {
	topos := []collector.CPUTopology{
		{ID: 0, MaxFreq: 4000000},
		{ID: 1, MaxFreq: 4000000},
	}
	pCores := collector.PCoreIDs(topos)
	if len(pCores) != 2 {
		t.Errorf("all-same-freq: PCoreIDs() = %v, want both CPUs", pCores)
	}
}
