package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/zenardi/gameperf/internal/collector"
)

// Snapshot holds a point-in-time collection of all metrics.
type Snapshot struct {
	Time        time.Time
	GPU         collector.GPUStat
	IRQs        []collector.IRQEntry
	CPUCount    int
	CPUStats    []collector.CPUStat
	CPUTopology []collector.CPUTopology
	GameProcs   []collector.ProcessInfo
}

// Collect gathers all metrics into a snapshot.
func Collect(gameNames []string) (Snapshot, error) {
	snap := Snapshot{Time: time.Now()}

	var err error

	snap.IRQs, snap.CPUCount, err = collector.CollectIRQs()
	if err != nil {
		return snap, fmt.Errorf("irq: %w", err)
	}

	snap.CPUStats, err = collector.CollectCPUStats()
	if err != nil {
		return snap, fmt.Errorf("cpu stats: %w", err)
	}

	snap.CPUTopology, err = collector.CollectCPUTopology()
	if err != nil {
		return snap, fmt.Errorf("cpu topology: %w", err)
	}

	if collector.GPUAvailable() {
		snap.GPU, err = collector.CollectGPU()
		if err != nil {
			return snap, fmt.Errorf("gpu: %w", err)
		}
	}

	snap.GameProcs, err = collector.FindGameProcesses(gameNames)
	if err != nil {
		return snap, fmt.Errorf("process: %w", err)
	}

	return snap, nil
}

// Analyze runs all diagnostic rules against the snapshot and returns findings.
func Analyze(snap Snapshot) []Finding {
	var findings []Finding

	findings = append(findings, checkIRQEcorePinning(snap)...)
	findings = append(findings, checkVRAMPressure(snap)...)
	findings = append(findings, checkGameNotRunning(snap)...)

	return findings
}

// checkIRQEcorePinning detects GPU IRQs routed to slow E-cores.
func checkIRQEcorePinning(snap Snapshot) []Finding {
	pCores := collector.PCoreIDs(snap.CPUTopology)
	if len(pCores) == 0 {
		return nil
	}
	pCoreSet := make(map[int]bool, len(pCores))
	for _, id := range pCores {
		pCoreSet[id] = true
	}
	maxPCore := pCores[len(pCores)-1]

	var findings []Finding
	targets := []string{"nvidia", "i915", "amdgpu", "radeon"}

	for _, entry := range snap.IRQs {
		nameLower := strings.ToLower(entry.Name)
		matched := false
		for _, t := range targets {
			if strings.Contains(nameLower, t) {
				matched = true
				break
			}
		}
		if !matched || entry.Total == 0 {
			continue
		}

		topCPU := entry.TopCPU()
		if pCoreSet[topCPU] {
			continue // already on a P-core
		}

		pct := float64(entry.PerCPU[topCPU]) / float64(entry.Total) * 100
		affinity, _ := collector.AffinityList(entry.Number)

		findings = append(findings, Finding{
			ID:       fmt.Sprintf("irq-ecore-%s", entry.Number),
			Severity: SeverityCritical,
			Title:    fmt.Sprintf("IRQ %s (%s) routed to E-core CPU%d", entry.Number, entry.Name, topCPU),
			Description: fmt.Sprintf(
				"%.1f%% of interrupts for %s (IRQ %s) are handled by CPU%d, which is a slow E-core. "+
					"This causes high kernel sys%% time on that core and direct frame stutter. "+
					"P-cores are CPU 0–%d on this system.",
				pct, entry.Name, entry.Number, topCPU, maxPCore,
			),
			Evidence: fmt.Sprintf("IRQ %s total=%d, CPU%d=%d (%.1f%%), affinity=%s",
				entry.Number, entry.Total, topCPU, entry.PerCPU[topCPU], pct, affinity),
			AutoFixable: true,
			AutoFixCmd: []string{
				"sh", "-c",
				fmt.Sprintf("echo 0-%d | tee /proc/irq/%s/smp_affinity_list", maxPCore, entry.Number),
			},
			ManualFix: fmt.Sprintf(
				"Run as root:\n  echo 0-%d | sudo tee /proc/irq/%s/smp_affinity_list\n\n"+
					"To persist across reboots, add to /etc/rc.local or create a udev rule.",
				maxPCore, entry.Number,
			),
		})
	}
	return findings
}

// checkVRAMPressure detects near-full VRAM which causes texture eviction stutters.
func checkVRAMPressure(snap Snapshot) []Finding {
	if snap.GPU.MemoryTotal == 0 {
		return nil
	}
	pct := snap.GPU.MemoryUsedPercent()
	if pct < 85 {
		return nil
	}

	severity := SeverityWarning
	desc := "VRAM usage is high"
	if pct >= 93 {
		severity = SeverityCritical
		desc = "VRAM is critically full"
	}

	return []Finding{{
		ID:       "vram-pressure",
		Severity: severity,
		Title:    fmt.Sprintf("VRAM at %.0f%% (%d / %d MiB)", pct, snap.GPU.MemoryUsed, snap.GPU.MemoryTotal),
		Description: fmt.Sprintf(
			"%s (%.0f%% used). The GPU must evict textures from VRAM to system RAM on every new scene, "+
				"causing multi-frame stalls. Frame Generation requires an additional ~1.5 GB and will make this worse.",
			desc, pct,
		),
		Evidence:    fmt.Sprintf("MemoryUsed=%d MiB, MemoryTotal=%d MiB", snap.GPU.MemoryUsed, snap.GPU.MemoryTotal),
		AutoFixable: false,
		InGameFix:   "Settings → Graphics → Texture Quality → lower by one step. Also disable Frame Generation if enabled.",
		ManualFix:   "Ensure Frame Generation is disabled in OptiScaler.ini: [FrameGen] Enabled=false",
	}}
}

// checkGameNotRunning warns if no game process was found.
func checkGameNotRunning(snap Snapshot) []Finding {
	if len(snap.GameProcs) > 0 {
		return nil
	}
	return []Finding{{
		ID:          "game-not-running",
		Severity:    SeverityInfo,
		Title:       "No game process detected",
		Description: "No matching game process was found. Metrics are from idle system state and may not reflect in-game conditions.",
		ManualFix:   "Launch the game and re-run gameperf monitor.",
	}}
}
