package volume

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeanhaley32/portable-claude-env/internal/constants"
)

// LinuxVolumeManager implements VolumeManager using cryptsetup/LUKS for Linux.
type LinuxVolumeManager struct{}

// NewLinuxVolumeManager creates a new Linux volume manager.
func NewLinuxVolumeManager() *LinuxVolumeManager {
	return &LinuxVolumeManager{}
}

func (l *LinuxVolumeManager) Bootstrap(config BootstrapConfig) error {
	// TODO: Implement LUKS volume creation
	// 1. dd if=/dev/zero of=<path> bs=1G count=<size>
	// 2. cryptsetup luksFormat <path>
	// 3. cryptsetup open <path> claude-env
	// 4. mkfs.ext4 /dev/mapper/claude-env
	// 5. cryptsetup close claude-env
	return fmt.Errorf("Linux bootstrap not yet implemented")
}

func (l *LinuxVolumeManager) Mount(volumePath, password string) (string, error) {
	// TODO: Implement LUKS mounting
	// 1. echo -n <password> | cryptsetup open <volumePath> claude-env -
	// 2. mkdir -p <mountPoint>
	// 3. mount /dev/mapper/claude-env <mountPoint>
	return "", fmt.Errorf("Linux mount not yet implemented")
}

func (l *LinuxVolumeManager) Unmount(mountPoint string) error {
	// TODO: Implement LUKS unmounting
	// 1. umount <mountPoint>
	// 2. cryptsetup close claude-env
	return fmt.Errorf("Linux unmount not yet implemented")
}

func (l *LinuxVolumeManager) Exists(volumePath string) bool {
	_, err := os.Stat(volumePath)
	return err == nil
}

func (l *LinuxVolumeManager) GetVolumePath(baseDir string) string {
	return filepath.Join(baseDir, constants.LinuxVolumeFile)
}

// IsMounted returns true if the volume is currently mounted.
func (l *LinuxVolumeManager) IsMounted() bool {
	// TODO: Check if /dev/mapper/claude-env exists and is mounted
	_, err := os.Stat(constants.LinuxMountPoint)
	return err == nil
}

// GetMountPoint returns the current mount point if mounted, empty string otherwise.
func (l *LinuxVolumeManager) GetMountPoint() string {
	if l.IsMounted() {
		return constants.LinuxMountPoint
	}
	return ""
}
