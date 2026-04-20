package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/collector"
)

const namespace = "gameperf"

// Metrics holds all Prometheus gauges for gameperf and the registry they belong to.
// Exported fields are exposed for testing via prometheus/testutil.
type Metrics struct {
	Registry *prometheus.Registry

	CPUUsage    *prometheus.GaugeVec
	CPUGovernor *prometheus.GaugeVec
	CPUThrottle *prometheus.GaugeVec
	GPUVRAMUsed  prometheus.Gauge
	GPUVRAMTotal prometheus.Gauge
	GPUVRAMPct   prometheus.Gauge
	RAMAvailable prometheus.Gauge
	SwapUsed     prometheus.Gauge
	VMMaxMapCount prometheus.Gauge
	FindingActive *prometheus.GaugeVec

	prevCPUStats []collector.CPUStat
}

// New creates and registers all gameperf metrics with a fresh isolated registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		CPUUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_usage_percent",
			Help:      "CPU usage percent per core (0–100), computed as delta between snapshots.",
		}, []string{"core"}),

		CPUGovernor: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_governor_info",
			Help:      "Current CPU scaling governor per core. Value is always 1; governor name is in the label.",
		}, []string{"core", "governor"}),

		CPUThrottle: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cpu_throttle_percent",
			Help:      "CPU throttle percent per core (0 = full speed, 100 = fully throttled).",
		}, []string{"core"}),

		GPUVRAMUsed: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gpu_vram_used_mib",
			Help:      "GPU VRAM used in MiB.",
		}),

		GPUVRAMTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gpu_vram_total_mib",
			Help:      "GPU VRAM total capacity in MiB.",
		}),

		GPUVRAMPct: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "gpu_vram_used_percent",
			Help:      "GPU VRAM used as a percentage of total capacity.",
		}),

		RAMAvailable: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ram_available_percent",
			Help:      "RAM available as a percentage of total physical memory.",
		}),

		SwapUsed: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "swap_used_percent",
			Help:      "Swap space used as a percentage of total swap.",
		}),

		VMMaxMapCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "vm_max_map_count",
			Help:      "Current value of vm.max_map_count (minimum 524288 required by many games).",
		}),

		FindingActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "finding_active",
			Help:      "Set to 1 when a diagnostic finding is currently active, 0 (absent) when resolved.",
		}, []string{"id", "severity"}),
	}

	reg.MustRegister(
		m.CPUUsage,
		m.CPUGovernor,
		m.CPUThrottle,
		m.GPUVRAMUsed,
		m.GPUVRAMTotal,
		m.GPUVRAMPct,
		m.RAMAvailable,
		m.SwapUsed,
		m.VMMaxMapCount,
		m.FindingActive,
	)

	return m
}

// UpdateFromSnapshot updates all gauges from the collected snapshot and findings.
// CPU usage is computed as a delta against the previous call — on the very first call
// no cpu_usage_percent metrics are emitted (no baseline yet).
func (m *Metrics) UpdateFromSnapshot(snap analyzer.Snapshot, findings []analyzer.Finding) {
	m.updateCPU(snap)
	m.updateGPU(snap)
	m.updateMemory(snap)
	m.updateFindings(findings)
}

func (m *Metrics) updateCPU(snap analyzer.Snapshot) {
	if len(m.prevCPUStats) == len(snap.CPUStats) {
		for i, curr := range snap.CPUStats {
			prev := m.prevCPUStats[i]
			usage := 100 - curr.IdlePercent(prev)
			m.CPUUsage.WithLabelValues(fmt.Sprintf("%d", curr.ID)).Set(usage)
		}
	}
	m.prevCPUStats = make([]collector.CPUStat, len(snap.CPUStats))
	copy(m.prevCPUStats, snap.CPUStats)

	// CPU governor — reset first to eliminate stale label combinations.
	m.CPUGovernor.Reset()
	for _, gov := range snap.CPUGovernors {
		m.CPUGovernor.WithLabelValues(fmt.Sprintf("%d", gov.ID), gov.Governor).Set(1)
	}

	// CPU throttle — no delta needed, computed from cur/max freq ratio.
	for _, freq := range snap.CPUFreqs {
		m.CPUThrottle.WithLabelValues(fmt.Sprintf("%d", freq.ID)).Set(freq.ThrottlePercent())
	}
}

func (m *Metrics) updateGPU(snap analyzer.Snapshot) {
	m.GPUVRAMUsed.Set(float64(snap.GPU.MemoryUsed))
	m.GPUVRAMTotal.Set(float64(snap.GPU.MemoryTotal))
	m.GPUVRAMPct.Set(snap.GPU.MemoryUsedPercent())
}

func (m *Metrics) updateMemory(snap analyzer.Snapshot) {
	m.RAMAvailable.Set(snap.MemInfo.AvailablePercent())
	m.SwapUsed.Set(snap.MemInfo.SwapUsedPercent())
	m.VMMaxMapCount.Set(float64(snap.VMMaxMapCount))
}

func (m *Metrics) updateFindings(findings []analyzer.Finding) {
	// Reset so resolved findings disappear rather than staying at 1.
	m.FindingActive.Reset()
	for _, f := range findings {
		m.FindingActive.WithLabelValues(f.ID, string(f.Severity)).Set(1)
	}
}

// Handler returns an http.Handler that serves all registered metrics in Prometheus
// text exposition format. Mount this at /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
