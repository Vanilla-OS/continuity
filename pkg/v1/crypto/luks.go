package crypto

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: LUKS2 helpers for Continuity repository devices.
*/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LUKSOptions configures CreateLUKSRepository.
type LUKSOptions struct {
	// FS is "ext4" or "btrfs". Empty defaults to ext4.
	FS string
	// Label is the LUKS2 header label (max 47 bytes; longer values are truncated).
	Label string
	// KeyFile is an optional path added as an extra keyslot.
	KeyFile string
}

// LUKSRepository holds the runtime state of an opened LUKS device.
type LUKSRepository struct {
	DevicePath string
	MountPath  string
	DeviceName string
}

func mapperName(devicePath string) string {
	return filepath.Base(devicePath) + "-continuity"
}

// CreateLUKSRepository formats devicePath as LUKS2 with the chosen filesystem
// and mounts it at mountPath. The returned LUKSRepository must be closed by
// the caller.
func CreateLUKSRepository(devicePath, mountPath, password string, opts LUKSOptions) (*LUKSRepository, error) {
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	fs := opts.FS
	if fs == "" {
		fs = "ext4"
	}
	if fs != "ext4" && fs != "btrfs" {
		return nil, fmt.Errorf("unsupported filesystem: %q (want ext4 or btrfs)", fs)
	}

	args := []string{"luksFormat", "--type", "luks2", "--batch-mode"}
	if opts.Label != "" {
		args = append(args, "--label", truncateLabel(opts.Label))
	}
	args = append(args, devicePath, "-")
	if out, err := runWithStdin(password, "cryptsetup", args...); err != nil {
		return nil, fmt.Errorf("LUKS format failed: %w\n%s", err, out)
	}

	if opts.KeyFile != "" {
		if out, err := runWithStdin(password, "cryptsetup", "luksAddKey", devicePath, opts.KeyFile); err != nil {
			return nil, fmt.Errorf("LUKS addKey failed: %w\n%s", err, out)
		}
	}

	name := mapperName(devicePath)
	if out, err := runWithStdin(password, "cryptsetup", "luksOpen", devicePath, name, "-"); err != nil {
		return nil, fmt.Errorf("LUKS open failed: %w\n%s", err, out)
	}
	mapped := filepath.Join("/dev/mapper", name)

	if err := makeFilesystem(fs, mapped); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", name).Run()
		return nil, err
	}

	if err := mount(mapped, mountPath); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", name).Run()
		return nil, err
	}

	return &LUKSRepository{
		DevicePath: devicePath,
		MountPath:  mountPath,
		DeviceName: name,
	}, nil
}

// OpenLUKSRepository unlocks an existing LUKS device and mounts it at mountPath.
func OpenLUKSRepository(devicePath, mountPath, password string) (*LUKSRepository, error) {
	name := mapperName(devicePath)
	if out, err := runWithStdin(password, "cryptsetup", "luksOpen", devicePath, name, "-"); err != nil {
		return nil, fmt.Errorf("LUKS open failed: %w\n%s", err, out)
	}
	mapped := filepath.Join("/dev/mapper", name)

	if err := mount(mapped, mountPath); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", name).Run()
		return nil, err
	}
	return &LUKSRepository{
		DevicePath: devicePath,
		MountPath:  mountPath,
		DeviceName: name,
	}, nil
}

// Close unmounts and locks the LUKS device.
func (r *LUKSRepository) Close() error {
	if err := exec.Command("umount", r.MountPath).Run(); err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}
	if err := exec.Command("cryptsetup", "luksClose", r.DeviceName).Run(); err != nil {
		return fmt.Errorf("LUKS close failed: %w", err)
	}
	return nil
}

// CloseDevice locks an already-unmounted (or never-mounted) LUKS device.
func CloseDevice(devicePath string) error {
	name := mapperName(devicePath)
	mapped := filepath.Join("/dev/mapper", name)
	if _, err := os.Stat(mapped); os.IsNotExist(err) {
		return nil
	}
	if err := exec.Command("cryptsetup", "luksClose", name).Run(); err != nil {
		return fmt.Errorf("LUKS close failed: %w", err)
	}
	return nil
}

// IsLUKSDevice reports whether devicePath contains a LUKS header.
func IsLUKSDevice(devicePath string) (bool, error) {
	err := exec.Command("cryptsetup", "isLuks", devicePath).Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("failed to check LUKS: %w", err)
}

// MapperPath returns the /dev/mapper path used when devicePath is opened.
func MapperPath(devicePath string) string {
	return filepath.Join("/dev/mapper", mapperName(devicePath))
}

func makeFilesystem(fs, devicePath string) error {
	var args []string
	switch fs {
	case "ext4":
		args = []string{"mkfs.ext4", "-F", devicePath}
	case "btrfs":
		args = []string{"mkfs.btrfs", "-f", devicePath}
	default:
		return fmt.Errorf("unsupported filesystem: %q", fs)
	}
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs %s failed: %w\n%s", fs, err, string(out))
	}
	return nil
}

func mount(source, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}
	if out, err := exec.Command("mount", source, target).CombinedOutput(); err != nil {
		return fmt.Errorf("mount failed: %w\n%s", err, string(out))
	}
	return nil
}

func runWithStdin(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// truncateLabel clamps a LUKS2 label to the 47-byte header limit.
func truncateLabel(s string) string {
	if len(s) <= 47 {
		return s
	}
	return s[:47]
}
