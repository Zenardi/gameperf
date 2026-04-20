package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CollectVMMaxMapCount reads vm.max_map_count from /proc/sys/vm/max_map_count.
func CollectVMMaxMapCount() (int64, error) {
	data, err := os.ReadFile("/proc/sys/vm/max_map_count")
	if err != nil {
		return 0, err
	}
	return ParseVMMaxMapCount(string(data))
}

// ParseVMMaxMapCount parses the raw content of /proc/sys/vm/max_map_count.
// Exposed for testing.
func ParseVMMaxMapCount(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse vm.max_map_count %q: %w", s, err)
	}
	return v, nil
}

// CPUGovernor holds the scaling governor for a single CPU core.
type CPUGovernor struct {
	ID       int
	Governor string
}

// CollectCPUGovernors reads the scaling governor for each online CPU.
func CollectCPUGovernors() ([]CPUGovernor, error) {
	return collectCPUGovernorsFrom("/sys/devices/system/cpu")
}

func collectCPUGovernorsFrom(sysfsRoot string) ([]CPUGovernor, error) {
	pattern := filepath.Join(sysfsRoot, "cpu[0-9]*", "cpufreq", "scaling_governor")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var governors []CPUGovernor
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Extract the cpu number from the directory component e.g. "cpu0"
		parts := strings.Split(filepath.ToSlash(path), "/")
		cpuID := -1
		for _, p := range parts {
			if strings.HasPrefix(p, "cpu") {
				n, err := strconv.Atoi(p[3:])
				if err == nil {
					cpuID = n
				}
			}
		}
		if cpuID < 0 {
			continue
		}
		governors = append(governors, CPUGovernor{
			ID:       cpuID,
			Governor: strings.TrimSpace(string(data)),
		})
	}
	sort.Slice(governors, func(i, j int) bool { return governors[i].ID < governors[j].ID })
	return governors, nil
}

// ParseCPUGovernor trims whitespace from a raw governor file content.
// Exposed for testing.
func ParseCPUGovernor(s string) string {
	return strings.TrimSpace(s)
}

// CPUFreqInfo holds the current and maximum CPU frequency for a single core.
type CPUFreqInfo struct {
	ID      int
	CurFreq int64 // kHz
	MaxFreq int64 // kHz
}

// ThrottlePercent returns how far below maximum the CPU is running (0 = no throttle).
func (f CPUFreqInfo) ThrottlePercent() float64 {
	if f.MaxFreq == 0 {
		return 0
	}
	throttle := 1 - float64(f.CurFreq)/float64(f.MaxFreq)
	if throttle < 0 {
		return 0
	}
	return throttle * 100
}

// CollectCPUFreqs reads the current and max frequency for each online CPU.
func CollectCPUFreqs() ([]CPUFreqInfo, error) {
	return collectCPUFreqsFrom("/sys/devices/system/cpu")
}

func collectCPUFreqsFrom(sysfsRoot string) ([]CPUFreqInfo, error) {
	pattern := filepath.Join(sysfsRoot, "cpu[0-9]*", "cpufreq", "scaling_cur_freq")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var freqs []CPUFreqInfo
	for _, curPath := range matches {
		curData, err := os.ReadFile(curPath)
		if err != nil {
			continue
		}
		maxPath := strings.Replace(curPath, "scaling_cur_freq", "cpuinfo_max_freq", 1)
		maxData, err := os.ReadFile(maxPath)
		if err != nil {
			continue
		}

		parts := strings.Split(filepath.ToSlash(curPath), "/")
		cpuID := -1
		for _, p := range parts {
			if strings.HasPrefix(p, "cpu") {
				n, err := strconv.Atoi(p[3:])
				if err == nil {
					cpuID = n
				}
			}
		}
		if cpuID < 0 {
			continue
		}

		cur, _ := strconv.ParseInt(strings.TrimSpace(string(curData)), 10, 64)
		max, _ := strconv.ParseInt(strings.TrimSpace(string(maxData)), 10, 64)
		freqs = append(freqs, CPUFreqInfo{ID: cpuID, CurFreq: cur, MaxFreq: max})
	}
	sort.Slice(freqs, func(i, j int) bool { return freqs[i].ID < freqs[j].ID })
	return freqs, nil
}
