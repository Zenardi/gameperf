package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/zenardi/gameperf/internal/collector"
)

// Snapshot holds a point-in-time collection of all metrics.
type Snapshot struct {
	Time              time.Time
	GPU               collector.GPUStat
	IRQs              []collector.IRQEntry
	CPUCount          int
	CPUStats          []collector.CPUStat
	CPUTopology       []collector.CPUTopology
	GameProcs         []collector.ProcessInfo
	MemInfo           collector.MemInfo
	THPMode           string
	VMMaxMapCount     int64
	CPUGovernors      []collector.CPUGovernor
	CPUFreqs          []collector.CPUFreqInfo
	IRQBalanceRunning bool
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

	// Optional metrics — skip silently on error (VM/container environments may lack sysfs)
	snap.MemInfo, _ = collector.CollectMemInfo()
	snap.THPMode, _ = collector.CollectTHPMode()
	if v, err := collector.CollectVMMaxMapCount(); err == nil {
		snap.VMMaxMapCount = v
	}
	snap.CPUGovernors, _ = collector.CollectCPUGovernors()
	snap.CPUFreqs, _ = collector.CollectCPUFreqs()

	// irqbalance is running if any process named "irqbalance" is found
	irqbProcs, _ := collector.FindGameProcesses([]string{"irqbalance"})
	snap.IRQBalanceRunning = len(irqbProcs) > 0

	return snap, nil
}

// Analyze runs all diagnostic rules against the snapshot and returns findings.
func Analyze(snap Snapshot) []Finding {
	var findings []Finding
	findings = append(findings, checkIRQEcorePinning(snap)...)
	findings = append(findings, checkVRAMPressure(snap)...)
	findings = append(findings, checkGameNotRunning(snap)...)
	findings = append(findings, checkCPUGovernor(snap)...)
	findings = append(findings, checkVMMaxMapCount(snap)...)
	findings = append(findings, checkTHP(snap)...)
	findings = append(findings, checkSwapPressure(snap)...)
	findings = append(findings, checkRAMPressure(snap)...)
	findings = append(findings, checkCPUThrottling(snap)...)
	findings = append(findings, checkIRQBalanceMissing(snap)...)
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

// checkCPUGovernor detects P-cores running on the powersave governor.
func checkCPUGovernor(snap Snapshot) []Finding {
	if len(snap.CPUGovernors) == 0 {
		return nil
	}
	pCores := collector.PCoreIDs(snap.CPUTopology)
	pCoreSet := make(map[int]bool, len(pCores))
	for _, id := range pCores {
		pCoreSet[id] = true
	}

	var powersave []int
	for _, g := range snap.CPUGovernors {
		if pCoreSet[g.ID] && g.Governor == "powersave" {
			powersave = append(powersave, g.ID)
		}
	}
	if len(powersave) == 0 {
		return nil
	}

	return []Finding{{
		ID:       "cpu-governor-powersave",
		Severity: SeverityCritical,
		Title:    fmt.Sprintf("%d P-core(s) using powersave CPU governor", len(powersave)),
		Description: "The powersave governor caps CPU frequency far below maximum, severely limiting game " +
			"performance and causing frame stutter whenever the engine needs a CPU burst. " +
			"Switch to performance or schedutil.",
		Evidence:    fmt.Sprintf("P-cores on powersave: %v", powersave),
		AutoFixable: true,
		AutoFixCmd:  []string{"sh", "-c", "echo performance | tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor"},
		ManualFix:   "echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor\n\nTo persist, install cpupower and set it in /etc/default/cpupower or your init system.",
	}}
}

// checkVMMaxMapCount detects a vm.max_map_count below the recommended minimum.
func checkVMMaxMapCount(snap Snapshot) []Finding {
	const recommended = 1048576
	if snap.VMMaxMapCount == 0 || snap.VMMaxMapCount >= recommended {
		return nil
	}
	return []Finding{{
		ID:       "vm-max-map-count",
		Severity: SeverityWarning,
		Title:    fmt.Sprintf("vm.max_map_count is %d (recommended ≥ %d)", snap.VMMaxMapCount, recommended),
		Description: "Many games (especially those using Vulkan or large texture streaming) require more " +
			"memory-mapped regions than the kernel default of 65530. A low value causes crashes or " +
			"stutters when the limit is hit mid-game.",
		Evidence:    fmt.Sprintf("vm.max_map_count = %d", snap.VMMaxMapCount),
		AutoFixable: true,
		AutoFixCmd:  []string{"sysctl", "-w", fmt.Sprintf("vm.max_map_count=%d", recommended)},
		ManualFix:   fmt.Sprintf("sudo sysctl -w vm.max_map_count=%d\n\nTo persist: echo 'vm.max_map_count=%d' | sudo tee /etc/sysctl.d/99-gameperf.conf", recommended, recommended),
	}}
}

// checkTHP detects transparent hugepage mode set to "always", which causes
// periodic allocation stalls during game runtime.
func checkTHP(snap Snapshot) []Finding {
	if snap.THPMode != "always" {
		return nil
	}
	return []Finding{{
		ID:       "thp-always",
		Severity: SeverityWarning,
		Title:    "Transparent Huge Pages set to 'always'",
		Description: "THP=always makes the kernel eagerly allocate 2 MB pages, causing periodic " +
			"compaction stalls that appear as random micro-stutters. 'madvise' lets the runtime " +
			"opt in only where beneficial (Wine/Proton already does this).",
		Evidence:    "THP mode = always",
		AutoFixable: true,
		AutoFixCmd:  []string{"sh", "-c", "echo madvise | tee /sys/kernel/mm/transparent_hugepage/enabled"},
		ManualFix:   "echo madvise | sudo tee /sys/kernel/mm/transparent_hugepage/enabled\n\nTo persist: add 'transparent_hugepage=madvise' to GRUB_CMDLINE_LINUX in /etc/default/grub.",
	}}
}

// checkSwapPressure detects high swap utilisation which causes severe I/O stalls.
func checkSwapPressure(snap Snapshot) []Finding {
	if snap.MemInfo.SwapTotal == 0 {
		return nil
	}
	pct := snap.MemInfo.SwapUsedPercent()
	if pct < 50 {
		return nil
	}
	severity := SeverityWarning
	if pct >= 80 {
		severity = SeverityCritical
	}
	return []Finding{{
		ID:       "swap-pressure",
		Severity: severity,
		Title:    fmt.Sprintf("Swap usage at %.0f%%", pct),
		Description: "High swap usage means the OS is paging memory to disk. Game textures and assets " +
			"being evicted to swap cause seconds-long stalls when they are needed again. Close other " +
			"applications to free RAM.",
		Evidence:    fmt.Sprintf("SwapUsed = %d MiB / %d MiB", (snap.MemInfo.SwapTotal-snap.MemInfo.SwapFree)/1024, snap.MemInfo.SwapTotal/1024),
		AutoFixable: false,
		ManualFix:   "Close memory-heavy background applications (browsers, VMs, etc.) before launching the game.",
	}}
}

// checkRAMPressure detects low available system RAM.
func checkRAMPressure(snap Snapshot) []Finding {
	const warnThresholdMiB = 2048
	if snap.MemInfo.MemTotal == 0 {
		return nil
	}
	availMiB := snap.MemInfo.AvailableMiB()
	if availMiB >= warnThresholdMiB {
		return nil
	}
	return []Finding{{
		ID:       "ram-pressure",
		Severity: SeverityWarning,
		Title:    fmt.Sprintf("Low available RAM: %d MiB", availMiB),
		Description: "Less than 2 GiB of RAM is available. The game may compete with the OS for memory, " +
			"causing texture eviction to swap and severe frame stutter.",
		Evidence:    fmt.Sprintf("MemAvailable = %d MiB, MemTotal = %d MiB", availMiB, snap.MemInfo.MemTotal/1024),
		AutoFixable: false,
		ManualFix:   "Close background applications to free RAM before launching the game.",
	}}
}

// checkCPUThrottling detects P-cores running significantly below their maximum frequency.
func checkCPUThrottling(snap Snapshot) []Finding {
	if len(snap.CPUFreqs) == 0 || len(snap.CPUTopology) == 0 {
		return nil
	}
	pCores := collector.PCoreIDs(snap.CPUTopology)
	pCoreSet := make(map[int]bool, len(pCores))
	for _, id := range pCores {
		pCoreSet[id] = true
	}

	const throttleThreshold = 30.0 // >30% below max = throttling
	var throttled []collector.CPUFreqInfo
	for _, f := range snap.CPUFreqs {
		if pCoreSet[f.ID] && f.ThrottlePercent() >= throttleThreshold {
			throttled = append(throttled, f)
		}
	}
	if len(throttled) == 0 {
		return nil
	}

	eg := throttled[0]
	return []Finding{{
		ID:       "cpu-throttling",
		Severity: SeverityWarning,
		Title:    fmt.Sprintf("%d P-core(s) throttled (e.g. CPU%d: %d / %d MHz)", len(throttled), eg.ID, eg.CurFreq/1000, eg.MaxFreq/1000),
		Description: "P-cores are running well below their rated frequency. This is usually caused by " +
			"thermal throttling (CPU too hot) or power limits. Lower in-game settings, improve case " +
			"airflow, or check thermal paste.",
		Evidence: fmt.Sprintf("CPU%d: cur=%d kHz, max=%d kHz (%.0f%% throttled)",
			eg.ID, eg.CurFreq, eg.MaxFreq, eg.ThrottlePercent()),
		AutoFixable: false,
		ManualFix:   "Check CPU temperature with: sensors\nIf > 95°C: repaste CPU, clean fans, reduce in-game settings.\nCheck power limits with: cat /sys/devices/system/cpu/cpu0/cpufreq/energy_performance_preference",
	}}
}

// checkIRQBalanceMissing warns when irqbalance is not running.
func checkIRQBalanceMissing(snap Snapshot) []Finding {
	if snap.IRQBalanceRunning {
		return nil
	}
	return []Finding{{
		ID:       "irqbalance-missing",
		Severity: SeverityWarning,
		Title:    "irqbalance is not running",
		Description: "irqbalance distributes hardware interrupt handling across CPU cores. Without it, " +
			"all IRQs default to CPU0, which becomes a bottleneck during heavy I/O (NVMe, network, GPU) " +
			"and causes frame stutter.",
		AutoFixable: true,
		AutoFixCmd:  []string{"systemctl", "enable", "--now", "irqbalance"},
		ManualFix:   "sudo systemctl enable --now irqbalance",
	}}
}
