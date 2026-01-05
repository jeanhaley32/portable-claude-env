//go:build darwin || linux

package docker

import "syscall"

// execSyscall replaces the current process with a new one.
// This function does not return on success.
func execSyscall(path string, args []string, env []string) error {
	return syscall.Exec(path, args, env)
}
