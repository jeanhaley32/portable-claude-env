package platform

import "runtime"

// OS represents a supported operating system.
type OS string

const (
	MacOS   OS = "darwin"
	Unknown OS = "unknown"
)

// Detect returns the current operating system.
func Detect() OS {
	switch runtime.GOOS {
	case "darwin":
		return MacOS
	default:
		return Unknown
	}
}

// IsMacOS returns true if running on macOS.
func IsMacOS() bool {
	return Detect() == MacOS
}

// IsSupported returns true if the current OS is supported.
// Currently only macOS is supported.
func IsSupported() bool {
	return Detect() == MacOS
}
