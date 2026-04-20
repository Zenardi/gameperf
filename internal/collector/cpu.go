package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// CPUStat holds a single CPU's usage counters from /proc/stat.
type CPUStat struct {
	ID      int
	User    int64
	Nice    int64
	System  int64
	Idle    int64
	IOWait  int64
	IRQ     int64
	SoftIRQ int64
}

// SysPercent returns the percentage of time spent in kernel mode.
func (s CPUStat) SysPercent(prev CPUStat) float64 {
	totalDiff := s.total() - prev.total()
	if totalDiff == 0 {
		return 0
	}
	sysDiff := (s.System + s.IRQ + s.SoftIRQ) - (prev.System + prev.IRQ + prev.SoftIRQ)
	return float64(sysDiff) / float64(totalDiff) * 100
}

// IdlePercent returns the percentage of idle time.
func (s CPUStat) IdlePercent(prev CPUStat) float64 {
	totalDiff := s.total() - prev.total()
	if totalDiff == 0 {
		return 100
	}
	idleDiff := s.Idle - prev.Idle
	return float64(idleDiff) / float64(totalDiff) * 100
}

func (s CPUStat) total() int64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ
}

// CollectCPUStats reads /proc/stat and returns per-CPU counters.
func CollectCPUStats() ([]CPUStat, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var stats []CPUStat
	scanner := bufio.NewScanner(f)
	cpuID := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") || line[:4] == "cpu " {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}
		stat := CPUStat{ID: cpuID}
		vals := []*int64{&stat.User, &stat.Nice, &stat.System, &stat.Idle, &stat.IOWait, &stat.IRQ, &stat.SoftIRQ}
		for i, v := range vals {
			*v, _ = strconv.ParseInt(parts[i+1], 10, 64)
		}
		stats = append(stats, stat)
		cpuID++
	}
	return stats, scanner.Err()
}

// CPUTopology holds basic topology info for a CPU core.
type CPUTopology struct {
	ID      int
	MaxFreq int64 // kHz
}

// CollectCPUTopology reads max frequencies from /sys to determine P-cores vs E-cores.
func CollectCPUTopology() ([]CPUTopology, error) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var topos []CPUTopology
	var current CPUTopology
	current.ID = -1

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "processor") {
			if current.ID >= 0 {
				topos = append(topos, current)
			}
			parts := strings.SplitN(line, ":", 2)
			current.ID, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			current.MaxFreq = 0
		}
		if strings.HasPrefix(line, "cpu MHz") {
			parts := strings.SplitN(line, ":", 2)
			mhz, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			current.MaxFreq = int64(mhz * 1000)
		}
	}
	if current.ID >= 0 {
		topos = append(topos, current)
	}
	return topos, scanner.Err()
}

// PCoreIDs returns the CPU IDs that appear to be P-cores (highest frequency cluster).
func PCoreIDs(topos []CPUTopology) []int {
	if len(topos) == 0 {
		return nil
	}
	var maxFreq int64
	for _, t := range topos {
		if t.MaxFreq > maxFreq {
			maxFreq = t.MaxFreq
		}
	}
	// P-cores are within 10% of max frequency
	threshold := int64(float64(maxFreq) * 0.90)
	var ids []int
	for _, t := range topos {
		if t.MaxFreq >= threshold {
			ids = append(ids, t.ID)
		}
	}
	return ids
}
