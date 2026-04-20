package collector

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// MemInfo holds key values from /proc/meminfo (all in kB).
type MemInfo struct {
	MemTotal     int64
	MemAvailable int64
	SwapTotal    int64
	SwapFree     int64
}

// SwapUsedPercent returns the percentage of swap currently in use.
func (m MemInfo) SwapUsedPercent() float64 {
	if m.SwapTotal == 0 {
		return 0
	}
	return float64(m.SwapTotal-m.SwapFree) / float64(m.SwapTotal) * 100
}

// AvailablePercent returns the percentage of RAM available.
func (m MemInfo) AvailablePercent() float64 {
	if m.MemTotal == 0 {
		return 100
	}
	return float64(m.MemAvailable) / float64(m.MemTotal) * 100
}

// AvailableMiB returns available RAM in MiB.
func (m MemInfo) AvailableMiB() int64 {
	return m.MemAvailable / 1024
}

// CollectMemInfo reads /proc/meminfo and returns a MemInfo.
func CollectMemInfo() (MemInfo, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	defer f.Close()
	return ParseMemInfo(f)
}

// ParseMemInfo parses /proc/meminfo format from any io.Reader.
// Exposed for testing.
func ParseMemInfo(r io.Reader) (MemInfo, error) {
	var info MemInfo
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		switch strings.TrimSuffix(parts[0], ":") {
		case "MemTotal":
			info.MemTotal = val
		case "MemAvailable":
			info.MemAvailable = val
		case "SwapTotal":
			info.SwapTotal = val
		case "SwapFree":
			info.SwapFree = val
		}
	}
	return info, scanner.Err()
}

// ParseTHPMode extracts the active transparent hugepage mode from the
// kernel's enabled file, where the active mode is surrounded by brackets:
// e.g. "always [madvise] never" -> "madvise".
func ParseTHPMode(content string) string {
	for _, field := range strings.Fields(content) {
		if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
			return strings.Trim(field, "[]")
		}
	}
	return strings.TrimSpace(content)
}

// CollectTHPMode reads the transparent hugepage mode from sysfs.
func CollectTHPMode() (string, error) {
	data, err := os.ReadFile("/sys/kernel/mm/transparent_hugepage/enabled")
	if err != nil {
		return "", err
	}
	return ParseTHPMode(string(data)), nil
}
