package collector_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

// buildFakeProc creates a minimal fake /proc tree under root.
// Each entry in procs defines one process:
//
//	pid    – numeric string
//	comm   – value written to /proc/<pid>/comm
//	exe    – symlink target for /proc/<pid>/exe (path string, not created on disk)
//	env    – null-separated key=value pairs written to /proc/<pid>/environ
func buildFakeProc(t *testing.T, procs []fakeProc) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range procs {
		dir := filepath.Join(root, p.pid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(p.comm+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Write environ as null-separated bytes
		if err := os.WriteFile(filepath.Join(dir, "environ"), []byte(p.env), 0o644); err != nil {
			t.Fatal(err)
		}
		// Write exe as a real file so os.Readlink can be replaced by os.ReadFile in tests.
		// We store the exe path in a plain file called "exe_target" (detect.go reads this
		// when procRoot != "/proc").
		if err := os.WriteFile(filepath.Join(dir, "exe_target"), []byte(p.exe), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type fakeProc struct {
	pid  string
	comm string
	exe  string
	env  string
}

func env(pairs ...string) string { return strings.Join(pairs, "\x00") }

// ── DetectGameProcesses ───────────────────────────────────────────────────────

func TestDetectGameProcesses_SteamAppId(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		{pid: "1001", comm: "GameBinary", exe: "/home/user/.local/share/Steam/steamapps/common/MyGame/GameBinary",
			env: env("HOME=/home/user", "SteamAppId=12345", "USER=user")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 process, got %d", len(procs))
	}
	if procs[0].Name != "GameBinary" {
		t.Errorf("expected GameBinary, got %q", procs[0].Name)
	}
	if procs[0].PID != 1001 {
		t.Errorf("expected PID 1001, got %d", procs[0].PID)
	}
}

func TestDetectGameProcesses_SteamappsExePath(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		// No SteamAppId in env, but exe is under steamapps/common
		{pid: "2001", comm: "NativeGame", exe: "/home/user/.steam/steam/steamapps/common/NativeGame/NativeGame",
			env: env("HOME=/home/user")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 process, got %d", len(procs))
	}
	if procs[0].Name != "NativeGame" {
		t.Errorf("expected NativeGame, got %q", procs[0].Name)
	}
}

func TestDetectGameProcesses_LutrisGame(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		{pid: "3001", comm: "game.exe", exe: "/home/user/Games/my-game/game.exe",
			env: env("HOME=/home/user", "LUTRIS_GAME_UUID=abc-123")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 lutris process, got %d", len(procs))
	}
}

func TestDetectGameProcesses_ExcludesSteamInfrastructure(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		// steamwebhelper — should be excluded even if it has steam env vars
		{pid: "500", comm: "steamwebhelper", exe: "/home/user/.local/share/Steam/ubuntu12_64/steamwebhelper",
			env: env("STEAM_RUNTIME=/some/path", "SteamClientLaunch=1")},
		// steam itself — excluded
		{pid: "501", comm: "steam", exe: "/home/user/.local/share/Steam/ubuntu12_32/steam",
			env: env("STEAM_RUNTIME=/some/path")},
		// actual game — kept
		{pid: "502", comm: "RealGame", exe: "/home/user/.local/share/Steam/steamapps/common/RealGame/RealGame",
			env: env("SteamAppId=99999")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 game process, got %d: %+v", len(procs), procs)
	}
	if procs[0].Name != "RealGame" {
		t.Errorf("expected RealGame, got %q", procs[0].Name)
	}
}

func TestDetectGameProcesses_ExcludesProtonInfrastructure(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		// pressure-vessel / reaper infrastructure — excluded
		{pid: "600", comm: "reaper", exe: "/home/user/.local/share/Steam/ubuntu12_32/reaper",
			env: env("SteamAppId=12345")},
		{pid: "601", comm: "pressure-vessel", exe: "/usr/lib/pressure-vessel/pressure-vessel-wrap",
			env: env("SteamAppId=12345")},
		// The actual Wine game process — kept
		{pid: "602", comm: "ff7rebirth_trial", exe: "/home/user/.local/share/Steam/steamapps/common/FFVII/ff7rebirth_trial.exe",
			env: env("SteamAppId=12345")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 process (game only), got %d: %+v", len(procs), procs)
	}
}

func TestDetectGameProcesses_NoneRunning(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		{pid: "100", comm: "bash", exe: "/bin/bash", env: env("HOME=/home/user")},
		{pid: "101", comm: "nvim", exe: "/usr/bin/nvim", env: env("HOME=/home/user")},
	})

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestDetectGameProcesses_SkipsNonNumericDirs(t *testing.T) {
	t.Parallel()
	root := buildFakeProc(t, []fakeProc{
		{pid: "1234", comm: "Game", exe: "/steamapps/common/Game/Game", env: env("SteamAppId=1")},
	})
	// Add a non-numeric dir that should be silently skipped
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}

	procs, err := collector.DetectGameProcesses(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 1 {
		t.Errorf("expected 1, got %d", len(procs))
	}
}
