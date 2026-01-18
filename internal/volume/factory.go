package volume

import (
	"fmt"
	"runtime"
)

// New creates a VolumeManager appropriate for the current operating system.
// Currently only macOS is supported.
func New() (VolumeManager, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewMacOSVolumeManager(), nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s (only macOS is supported)", runtime.GOOS)
	}
}
