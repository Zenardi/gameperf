package collector

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// IRQEntry holds the interrupt counts per CPU for a single IRQ line.
type IRQEntry struct {
	Number string
	Name   string
	PerCPU []int64
	Total  int64
}

// CollectIRQs reads /proc/interrupts and returns all IRQ entries.
func CollectIRQs() ([]IRQEntry, int, error) {
	f, err := os.Open("/proc/interrupts")
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	return ParseIRQs(f)
}

// ParseIRQs parses the /proc/interrupts format from any io.Reader.
// Exposed for testing.
func ParseIRQs(r io.Reader) ([]IRQEntry, int, error) {
	scanner := bufio.NewScanner(r)

	// First line is the CPU header — count CPUs.
	scanner.Scan()
	cpuCount := len(strings.Fields(scanner.Text()))

	var entries []IRQEntry
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		irqNum := strings.TrimSuffix(parts[0], ":")
		entry := IRQEntry{Number: irqNum}

		var total int64
		perCPU := make([]int64, cpuCount)
		for i := 0; i < cpuCount && i+1 < len(parts); i++ {
			v, err := strconv.ParseInt(parts[i+1], 10, 64)
			if err != nil {
				break
			}
			perCPU[i] = v
			total += v
		}
		entry.PerCPU = perCPU
		entry.Total = total

		// Name is the last non-numeric field.
		if len(parts) > cpuCount+1 {
			entry.Name = strings.Join(parts[cpuCount+1:], " ")
		}

		entries = append(entries, entry)
	}

	return entries, cpuCount, scanner.Err()
}

// FindIRQByName returns IRQ entries whose name contains the given substring (case-insensitive).
func FindIRQByName(entries []IRQEntry, substr string) []IRQEntry {
	substr = strings.ToLower(substr)
	var result []IRQEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), substr) {
			result = append(result, e)
		}
	}
	return result
}

// TopCPU returns the CPU index that has received the most interrupts for this IRQ.
func (e IRQEntry) TopCPU() int {
	top := 0
	for i, v := range e.PerCPU {
		if v > e.PerCPU[top] {
			top = i
		}
	}
	return top
}

// AffinityList reads the current smp_affinity_list for an IRQ number.
func AffinityList(irqNum string) (string, error) {
	data, err := os.ReadFile("/proc/irq/" + irqNum + "/smp_affinity_list")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
