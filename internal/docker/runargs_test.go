package docker

import (
	"strings"
	"testing"
)

// argValue returns the argument immediately following flag in args, or "".
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func TestBuildRunArgs_AppliesDefaultResourceBounds(t *testing.T) {
	cfg := ContainerConfig{
		ImageName:     "claude-capsule:latest",
		ContainerName: "claude-capsule",
	}
	args := buildRunArgs(cfg, "vol", "ws")

	if got := argValue(args, "--memory"); got != DefaultMemoryLimit {
		t.Errorf("--memory = %q, want default %q", got, DefaultMemoryLimit)
	}
	if got := argValue(args, "--pids-limit"); got != DefaultPidsLimit {
		t.Errorf("--pids-limit = %q, want default %q", got, DefaultPidsLimit)
	}
	// CPU default is unset -> flag must be absent.
	if DefaultCPULimit == "" && hasFlag(args, "--cpus") {
		t.Error("--cpus should be absent when DefaultCPULimit is empty")
	}
}

func TestBuildRunArgs_HonorsOverrides(t *testing.T) {
	cfg := ContainerConfig{
		ImageName:     "img",
		ContainerName: "c",
		MemoryLimit:   "2g",
		PidsLimit:     "128",
		CPULimit:      "1.5",
	}
	args := buildRunArgs(cfg, "vol", "ws")

	if got := argValue(args, "--memory"); got != "2g" {
		t.Errorf("--memory = %q, want 2g", got)
	}
	if got := argValue(args, "--pids-limit"); got != "128" {
		t.Errorf("--pids-limit = %q, want 128", got)
	}
	if got := argValue(args, "--cpus"); got != "1.5" {
		t.Errorf("--cpus = %q, want 1.5", got)
	}
}

func TestBuildRunArgs_PreservesCoreShape(t *testing.T) {
	cfg := ContainerConfig{ImageName: "myimg", ContainerName: "myctr"}
	args := buildRunArgs(cfg, "volmount", "wsmount")

	if len(args) == 0 || args[0] != "run" {
		t.Fatalf("args must start with run, got %v", args)
	}
	if !hasFlag(args, "-d") {
		t.Error("missing -d")
	}
	if got := argValue(args, "--name"); got != "myctr" {
		t.Errorf("--name = %q, want myctr", got)
	}
	// Image and the keep-alive tail command must remain the trailing args, in order.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--entrypoint tail myimg -f /dev/null") {
		t.Errorf("trailing entrypoint/image/keepalive shape changed: %v", args)
	}
	// Both mounts must be present.
	if !hasFlag(args, "volmount") || !hasFlag(args, "wsmount") {
		t.Errorf("mounts missing from args: %v", args)
	}
}
