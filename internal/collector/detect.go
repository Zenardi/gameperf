package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// infraProcessNames are Steam/Proton infrastructure process names that should
// never be reported as games even when they carry a SteamAppId env var.
var infraProcessNames = map[string]bool{
	"steam":               true,
	"steamwebhelper":      true,
	"steam-runtime":       true,
	"reaper":              true,
	"pressure-vessel":     true,
	"steam-runtime-launc": true, // truncated comm (kernel caps at 15 chars)
	"SteamLinuxRuntime":   true,
	"slr":                 true,
}

// DetectGameProcesses auto-detects running game processes by scanning procRoot
// (normally "/proc"). It returns processes that match any of:
//
//  1. SteamAppId environment variable — set by Steam for every launched game
//  2. Executable path under steamapps/common/ — native/Proton Steam games
//  3. LUTRIS_GAME_UUID environment variable — games launched via Lutris
//
// Infrastructure processes (steam, steamwebhelper, reaper, pressure-vessel, …)
// are excluded even when they carry a SteamAppId.
//
// Pass procRoot = "/proc" for production use; pass a temp dir in tests.
func DetectGameProcesses(procRoot string) ([]ProcessInfo, error) {
	entries, err := os.ReadDir(procRoot)
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
			continue // skip non-numeric dirs like "self", "thread-self"
		}

		pidDir := filepath.Join(procRoot, e.Name())

		comm, err := readComm(pidDir)
		if err != nil {
			continue
		}

		if infraProcessNames[comm] {
			continue
		}

		exe := readExe(pidDir, procRoot)

		if isInfraExe(exe) {
			continue
		}

		environ, err := readEnviron(pidDir)
		if err != nil {
			continue
		}

		if isGame(exe, environ) {
			found = append(found, ProcessInfo{PID: pid, Name: comm, Exe: exe})
		}
	}
	return found, nil
}

// isGame returns true when the process looks like a game based on its exe path
// or environment variables.
func isGame(exe string, environ map[string]string) bool {
	if _, ok := environ["SteamAppId"]; ok {
		return true
	}
	if _, ok := environ["LUTRIS_GAME_UUID"]; ok {
		return true
	}
	if strings.Contains(exe, "steamapps/common/") {
		return true
	}
	return false
}

// isInfraExe returns true for exe paths that belong to Steam/Proton
// infrastructure rather than an actual game.
func isInfraExe(exe string) bool {
	infraPatterns := []string{
		"pressure-vessel",
		"steam-runtime",
		"ubuntu12_32/steam",
		"ubuntu12_64/steam",
		"ubuntu12_32/reaper",
		"ubuntu12_64/reaper",
	}
	lower := strings.ToLower(exe)
	for _, p := range infraPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func readComm(pidDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(pidDir, "comm"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readExe reads the exe path. Under real /proc this is a symlink; in tests we
// store it as a plain file called "exe_target" to avoid needing symlink perms.
func readExe(pidDir, procRoot string) string {
	// Test mode: use "exe_target" plain file
	if procRoot != "/proc" {
		data, err := os.ReadFile(filepath.Join(pidDir, "exe_target"))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	// Production: follow /proc/<pid>/exe symlink
	target, err := os.Readlink(filepath.Join(pidDir, "exe"))
	if err != nil {
		return ""
	}
	return target
}

// readEnviron parses /proc/<pid>/environ (null-separated key=value pairs)
// into a map.
func readEnviron(pidDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(pidDir, "environ"))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, pair := range strings.Split(string(data), "\x00") {
		idx := strings.IndexByte(pair, '=')
		if idx < 0 {
			continue
		}
		result[pair[:idx]] = pair[idx+1:]
	}
	return result, nil
}
