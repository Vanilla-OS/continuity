package repoinit

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Orchestrates initialization, unlock and lock of Continuity
repositories living on external or internal block devices.
*/

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vanilla-os/continuity/pkg/v1/crypto"
	"github.com/vanilla-os/continuity/pkg/v1/devices"
	"github.com/vanilla-os/sdk/pkg/v1/backup"
)

// MountBase is the tmpfs directory under which Continuity repositories are
// mounted at runtime.
const MountBase = "/run/continuity"

// InitOptions configures InitRepository.
type InitOptions struct {
	DevicePath string
	Label      string
	Encrypt    bool
	// FS is "ext4" or "btrfs". Empty defaults to ext4.
	FS       string
	Password string
}

// UnlockOptions configures UnlockRepository.
type UnlockOptions struct {
	DevicePath string
	Password   string
}

// Repository represents an opened Continuity repository on a device.
type Repository struct {
	DevicePath string
	MountPath  string
	Marker     devices.RepoMarker
	LUKS       bool
	luks       *crypto.LUKSRepository
}

// InitRepository formats DevicePath as a Continuity repository.
// Any existing data on the device is destroyed.
func InitRepository(opts InitOptions) (*Repository, error) {
	if opts.DevicePath == "" {
		return nil, fmt.Errorf("device path is required")
	}
	if _, err := os.Stat(opts.DevicePath); err != nil {
		return nil, fmt.Errorf("device not accessible: %w", err)
	}
	fs := opts.FS
	if fs == "" {
		fs = "ext4"
	}
	if fs != "ext4" && fs != "btrfs" {
		return nil, fmt.Errorf("unsupported filesystem: %q", fs)
	}
	if opts.Encrypt && opts.Password == "" {
		return nil, fmt.Errorf("password is required when Encrypt is true")
	}

	uuid := newRepoUUID()
	mountPath := mountPathFor(uuid)
	createdAt := time.Now().UTC().Format(time.RFC3339)

	marker := devices.RepoMarker{
		Version:    devices.CurrentMarkerVersion,
		Label:      opts.Label,
		UUID:       uuid,
		CreatedAt:  createdAt,
		DeviceHint: opts.DevicePath,
	}

	if opts.Encrypt {
		return initEncrypted(opts, fs, mountPath, marker)
	}
	return initPlain(opts, fs, mountPath, marker)
}

func initEncrypted(opts InitOptions, fs, mountPath string, marker devices.RepoMarker) (*Repository, error) {
	luksOpts := crypto.LUKSOptions{
		FS:    fs,
		Label: opts.Label,
	}
	luksRepo, err := crypto.CreateLUKSRepository(opts.DevicePath, mountPath, opts.Password, luksOpts)
	if err != nil {
		return nil, err
	}
	tok := crypto.ContinuityToken{
		Version:   devices.CurrentMarkerVersion,
		Label:     opts.Label,
		UUID:      marker.UUID,
		CreatedAt: marker.CreatedAt,
	}
	if err := crypto.SetContinuityToken(opts.DevicePath, tok); err != nil {
		_ = luksRepo.Close()
		return nil, err
	}
	if err := layoutRepository(mountPath, marker); err != nil {
		_ = luksRepo.Close()
		return nil, err
	}
	return &Repository{
		DevicePath: opts.DevicePath,
		MountPath:  mountPath,
		Marker:     marker,
		LUKS:       true,
		luks:       luksRepo,
	}, nil
}

func initPlain(opts InitOptions, fs, mountPath string, marker devices.RepoMarker) (*Repository, error) {
	if out, err := mkfs(fs, opts.DevicePath); err != nil {
		return nil, fmt.Errorf("mkfs %s failed: %w\n%s", fs, err, out)
	}
	if err := mountAt(opts.DevicePath, mountPath); err != nil {
		return nil, err
	}
	if err := layoutRepository(mountPath, marker); err != nil {
		_ = exec.Command("umount", mountPath).Run()
		return nil, err
	}
	return &Repository{
		DevicePath: opts.DevicePath,
		MountPath:  mountPath,
		Marker:     marker,
		LUKS:       false,
	}, nil
}

// UnlockRepository opens an existing Continuity repository under MountBase.
// Password is required for LUKS devices and ignored otherwise.
func UnlockRepository(opts UnlockOptions) (*Repository, error) {
	if opts.DevicePath == "" {
		return nil, fmt.Errorf("device path is required")
	}
	isLUKS, err := crypto.IsLUKSDevice(opts.DevicePath)
	if err != nil {
		return nil, err
	}

	if isLUKS {
		if opts.Password == "" {
			return nil, fmt.Errorf("password is required to unlock LUKS device")
		}
		uuid := repoUUIDFromLUKS(opts.DevicePath)
		mountPath := mountPathFor(uuid)
		luksRepo, err := crypto.OpenLUKSRepository(opts.DevicePath, mountPath, opts.Password)
		if err != nil {
			return nil, err
		}
		marker, ok, err := devices.LoadMarker(mountPath)
		if err != nil {
			_ = luksRepo.Close()
			return nil, fmt.Errorf("read repository marker: %w", err)
		}
		if !ok {
			_ = luksRepo.Close()
			return nil, fmt.Errorf("device is LUKS but not a Continuity repository (no marker)")
		}
		return &Repository{
			DevicePath: opts.DevicePath,
			MountPath:  mountPath,
			Marker:     marker,
			LUKS:       true,
			luks:       luksRepo,
		}, nil
	}

	uuid := newRepoUUID()
	mountPath := mountPathFor(uuid)
	if err := mountAt(opts.DevicePath, mountPath); err != nil {
		return nil, err
	}
	marker, ok, err := devices.LoadMarker(mountPath)
	if err != nil {
		_ = exec.Command("umount", mountPath).Run()
		return nil, fmt.Errorf("read repository marker: %w", err)
	}
	if !ok {
		_ = exec.Command("umount", mountPath).Run()
		return nil, fmt.Errorf("device does not contain a Continuity repository")
	}
	return &Repository{
		DevicePath: opts.DevicePath,
		MountPath:  mountPath,
		Marker:     marker,
		LUKS:       false,
	}, nil
}

// Close unmounts the repository and, for LUKS devices, also locks it.
func (r *Repository) Close() error {
	if r.LUKS && r.luks != nil {
		return r.luks.Close()
	}
	if r.MountPath == "" {
		return nil
	}
	if err := exec.Command("umount", r.MountPath).Run(); err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}
	return nil
}

// LockDevice unmounts and (for LUKS) closes devicePath. Safe on locked devices.
func LockDevice(devicePath string) error {
	isLUKS, err := crypto.IsLUKSDevice(devicePath)
	if err != nil {
		return err
	}
	if isLUKS {
		mapped := crypto.MapperPath(devicePath)
		_ = unmountTargetsOf(mapped)
		return crypto.CloseDevice(devicePath)
	}
	return unmountTargetsOf(devicePath)
}

// layoutRepository creates the SDK repository directories and writes the marker.
func layoutRepository(mountPath string, marker devices.RepoMarker) error {
	if _, err := backup.OpenRepository(mountPath); err != nil {
		return fmt.Errorf("init repository layout: %w", err)
	}
	if err := devices.WriteMarker(mountPath, marker); err != nil {
		return err
	}
	return nil
}

func mountPathFor(uuid string) string {
	return filepath.Join(MountBase, uuid)
}

func newRepoUUID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func repoUUIDFromLUKS(devicePath string) string {
	if ok, tok, err := crypto.HasContinuityToken(devicePath); err == nil && ok && tok.UUID != "" {
		return tok.UUID
	}
	return newRepoUUID()
}

func mkfs(fs, devicePath string) (string, error) {
	var args []string
	switch fs {
	case "ext4":
		args = []string{"mkfs.ext4", "-F", devicePath}
	case "btrfs":
		args = []string{"mkfs.btrfs", "-f", devicePath}
	default:
		return "", fmt.Errorf("unsupported filesystem: %q", fs)
	}
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return string(out), err
}

func mountAt(source, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}
	if out, err := exec.Command("mount", source, target).CombinedOutput(); err != nil {
		return fmt.Errorf("mount failed: %w\n%s", err, string(out))
	}
	return nil
}

// unmountTargetsOf unmounts every mount whose source matches.
func unmountTargetsOf(source string) error {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return fmt.Errorf("read mountinfo: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// mountinfo: ... - <fstype> <source> <opts>
		dash := -1
		for i, f := range fields {
			if f == "-" {
				dash = i
				break
			}
		}
		if dash == -1 || dash+2 >= len(fields) {
			continue
		}
		mountSrc := fields[dash+2]
		mountTarget := fields[4]
		if mountSrc != source {
			continue
		}
		if err := exec.Command("umount", mountTarget).Run(); err != nil {
			return fmt.Errorf("umount %s: %w", mountTarget, err)
		}
	}
	return nil
}
