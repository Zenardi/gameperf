package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// ProcessInfo holds basic info about a running process.
type ProcessInfo struct {
	PID  int
	Name string
	Exe  string
}

// FindGameProcesses scans /proc for processes whose executable name matches
// any of the provided substrings (case-insensitive).
func FindGameProcesses(nameSubstrings []string) ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var found []ProcessInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		commBytes, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(commBytes))

		for _, substr := range nameSubstrings {
			if strings.Contains(strings.ToLower(comm), strings.ToLower(substr)) {
				exe, _ := os.Readlink("/proc/" + e.Name() + "/exe")
				found = append(found, ProcessInfo{PID: pid, Name: comm, Exe: exe})
				break
			}
		}
	}
	return found, nil
}

// ProcessFDCount returns the number of open file descriptors for a PID.
func ProcessFDCount(pid int) (int, error) {
	entries, err := os.ReadDir("/proc/" + strconv.Itoa(pid) + "/fd")
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// ProcessMemMiB returns the RSS memory usage of a process in MiB.
func ProcessMemMiB(pid int) (int64, error) {
	f, err := os.Open("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				kb, _ := strconv.ParseInt(parts[1], 10, 64)
				return kb / 1024, nil
			}
		}
	}
	return 0, nil
}
