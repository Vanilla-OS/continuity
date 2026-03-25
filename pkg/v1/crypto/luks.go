package crypto

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LUKSRepository struct {
	DevicePath string
	MountPath  string
	DeviceName string
}

func CreateLUKSRepository(devicePath, mountPath, password string) (*LUKSRepository, error) {
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	deviceName := filepath.Base(devicePath) + "-continuity"

	cmd := exec.Command("cryptsetup", "luksFormat", "--type", "luks2", devicePath, "-")
	cmd.Stdin = strings.NewReader(password)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("LUKS format failed: %w\n%s", err, string(output))
	}

	cmd = exec.Command("cryptsetup", "luksOpen", devicePath, deviceName, "-")
	cmd.Stdin = strings.NewReader(password)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("LUKS open failed: %w\n%s", err, string(output))
	}

	mappedDevice := filepath.Join("/dev/mapper", deviceName)

	cmd = exec.Command("mkfs.ext4", "-F", mappedDevice)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", deviceName).Run()
		return nil, fmt.Errorf("mkfs failed: %w\n%s", err, string(output))
	}

	if err := os.MkdirAll(mountPath, 0755); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", deviceName).Run()
		return nil, fmt.Errorf("failed to create mount point: %w", err)
	}

	cmd = exec.Command("mount", mappedDevice, mountPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", deviceName).Run()
		return nil, fmt.Errorf("mount failed: %w\n%s", err, string(output))
	}

	return &LUKSRepository{
		DevicePath: devicePath,
		MountPath:  mountPath,
		DeviceName: deviceName,
	}, nil
}

func OpenLUKSRepository(devicePath, mountPath, password string) (*LUKSRepository, error) {
	deviceName := filepath.Base(devicePath) + "-continuity"

	cmd := exec.Command("cryptsetup", "luksOpen", devicePath, deviceName, "-")
	cmd.Stdin = strings.NewReader(password)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("LUKS open failed: %w\n%s", err, string(output))
	}

	mappedDevice := filepath.Join("/dev/mapper", deviceName)

	if err := os.MkdirAll(mountPath, 0755); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", deviceName).Run()
		return nil, fmt.Errorf("failed to create mount point: %w", err)
	}

	cmd = exec.Command("mount", mappedDevice, mountPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = exec.Command("cryptsetup", "luksClose", deviceName).Run()
		return nil, fmt.Errorf("mount failed: %w\n%s", err, string(output))
	}

	return &LUKSRepository{
		DevicePath: devicePath,
		MountPath:  mountPath,
		DeviceName: deviceName,
	}, nil
}

func (r *LUKSRepository) Close() error {
	if err := exec.Command("umount", r.MountPath).Run(); err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}

	if err := exec.Command("cryptsetup", "luksClose", r.DeviceName).Run(); err != nil {
		return fmt.Errorf("LUKS close failed: %w", err)
	}

	return nil
}

func IsLUKSDevice(devicePath string) (bool, error) {
	cmd := exec.Command("cryptsetup", "isLuks", devicePath)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to check LUKS: %w", err)
}
