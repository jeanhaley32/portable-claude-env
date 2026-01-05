package platform

import "runtime"

// OS represents a supported operating system.
type OS string

const (
	MacOS   OS = "darwin"
	Linux   OS = "linux"
	Unknown OS = "unknown"
)

// Detect returns the current operating system.
func Detect() OS {
	switch runtime.GOOS {
	case "darwin":
		return MacOS
	case "linux":
		return Linux
	default:
		return Unknown
	}
}

// IsMacOS returns true if running on macOS.
func IsMacOS() bool {
	return Detect() == MacOS
}

// IsLinux returns true if running on Linux.
func IsLinux() bool {
	return Detect() == Linux
}

// IsSupported returns true if the current OS is supported.
func IsSupported() bool {
	os := Detect()
	return os == MacOS || os == Linux
}
