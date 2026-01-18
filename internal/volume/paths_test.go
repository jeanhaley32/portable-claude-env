package volume

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
)

func TestPathResolver_GetGlobalVolumeDir(t *testing.T) {
	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, constants.CapsuleConfigDir, constants.VolumesSubdir)

	got := resolver.GetGlobalVolumeDir()
	if got != expected {
		t.Errorf("GetGlobalVolumeDir() = %v, want %v", got, expected)
	}
}

func TestPathResolver_GetDefaultVolumePath(t *testing.T) {
	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, constants.CapsuleConfigDir, constants.VolumesSubdir, constants.MacOSVolumeFile)

	got := resolver.GetDefaultVolumePath()
	if got != expected {
		t.Errorf("GetDefaultVolumePath() = %v, want %v", got, expected)
	}
}

func TestPathResolver_GetLocalVolumePath(t *testing.T) {
	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	cwd := "/some/project/dir"
	expected := filepath.Join(cwd, constants.MacOSVolumeFile)

	got := resolver.GetLocalVolumePath(cwd)
	if got != expected {
		t.Errorf("GetLocalVolumePath(%v) = %v, want %v", cwd, got, expected)
	}
}

func TestPathResolver_ResolveVolumePath_ExplicitPath(t *testing.T) {
	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	explicitPath := "/custom/path/volume.sparseimage"
	cwd := "/some/project/dir"

	volumePath, exists := resolver.ResolveVolumePath(explicitPath, cwd)
	if volumePath != explicitPath {
		t.Errorf("ResolveVolumePath() volumePath = %v, want %v", volumePath, explicitPath)
	}
	if exists {
		t.Errorf("ResolveVolumePath() exists = true for non-existent path")
	}
}

func TestPathResolver_ResolveVolumePath_LocalVolume(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "capsule-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a local volume file
	localVolumePath := filepath.Join(tmpDir, constants.MacOSVolumeFile)
	if err := os.WriteFile(localVolumePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create local volume file: %v", err)
	}

	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	// Should find local volume
	volumePath, exists := resolver.ResolveVolumePath("", tmpDir)
	if volumePath != localVolumePath {
		t.Errorf("ResolveVolumePath() volumePath = %v, want %v", volumePath, localVolumePath)
	}
	if !exists {
		t.Errorf("ResolveVolumePath() exists = false, want true")
	}
}

func TestPathResolver_ResolveVolumePath_FallbackToGlobal(t *testing.T) {
	// Create a temporary directory with no local volume
	tmpDir, err := os.MkdirTemp("", "capsule-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	// Should fall back to global path
	volumePath, _ := resolver.ResolveVolumePath("", tmpDir)
	expected := resolver.GetDefaultVolumePath()
	if volumePath != expected {
		t.Errorf("ResolveVolumePath() volumePath = %v, want %v", volumePath, expected)
	}
}

func TestPathResolver_ResolveVolumePathStrict_NotFound(t *testing.T) {
	// Create a temporary directory with no volume
	tmpDir, err := os.MkdirTemp("", "capsule-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	resolver, err := NewPathResolver()
	if err != nil {
		t.Fatalf("NewPathResolver() error = %v", err)
	}

	// Should return error
	_, err = resolver.ResolveVolumePathStrict("", tmpDir)
	if err == nil {
		t.Error("ResolveVolumePathStrict() expected error, got nil")
	}

	// Error should be VolumeNotFoundError
	notFoundErr, ok := err.(*VolumeNotFoundError)
	if !ok {
		t.Errorf("ResolveVolumePathStrict() error type = %T, want *VolumeNotFoundError", err)
	}

	// Error message should mention both locations
	errMsg := notFoundErr.Error()
	if !contains(errMsg, "local") || !contains(errMsg, "global") {
		t.Errorf("VolumeNotFoundError.Error() should mention both local and global paths, got: %s", errMsg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
