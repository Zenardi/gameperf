package collector

import (
	"os/exec"
	"strconv"
	"strings"
)

// GPUStat holds a snapshot of NVIDIA GPU metrics.
type GPUStat struct {
	UtilizationGPU    int     // %
	UtilizationMemory int     // %
	PowerDraw         float64 // W
	ClockGraphics     int     // MHz
	MemoryUsed        int64   // MiB
	MemoryTotal       int64   // MiB
	DriverVersion     string
}

// MemoryUsedPercent returns VRAM utilization as a percentage.
func (g GPUStat) MemoryUsedPercent() float64 {
	if g.MemoryTotal == 0 {
		return 0
	}
	return float64(g.MemoryUsed) / float64(g.MemoryTotal) * 100
}

// CollectGPU queries nvidia-smi for current GPU metrics.
func CollectGPU() (GPUStat, error) {
	query := "utilization.gpu,utilization.memory,power.draw,clocks.current.graphics,memory.used,memory.total,driver_version"
	out, err := exec.Command("nvidia-smi",
		"--query-gpu="+query,
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return GPUStat{}, err
	}
	return ParseGPUOutput(string(out))
}

// ParseGPUOutput parses a single nvidia-smi CSV line into a GPUStat.
// Exposed for testing.
func ParseGPUOutput(output string) (GPUStat, error) {
	parts := strings.Split(strings.TrimSpace(output), ", ")
	if len(parts) < 7 {
		parts = strings.Split(strings.TrimSpace(output), ",")
	}
	if len(parts) < 7 {
		return GPUStat{}, nil
	}

	stat := GPUStat{}
	stat.UtilizationGPU, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	stat.UtilizationMemory, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	pw, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	stat.PowerDraw = pw
	stat.ClockGraphics, _ = strconv.Atoi(strings.TrimSpace(parts[3]))
	stat.MemoryUsed, _ = strconv.ParseInt(strings.TrimSpace(parts[4]), 10, 64)
	stat.MemoryTotal, _ = strconv.ParseInt(strings.TrimSpace(parts[5]), 10, 64)
	stat.DriverVersion = strings.TrimSpace(parts[6])
	return stat, nil
}

// GPUAvailable returns true if nvidia-smi is reachable.
func GPUAvailable() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}
