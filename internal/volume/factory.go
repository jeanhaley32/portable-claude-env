package volume

import (
	"fmt"
	"runtime"
)

// New creates a VolumeManager appropriate for the current operating system.
func New() (VolumeManager, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewMacOSVolumeManager(), nil
	case "linux":
		return NewLinuxVolumeManager(), nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s (use WSL2 on Windows)", runtime.GOOS)
	}
}
