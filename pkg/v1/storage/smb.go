package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: SMB/CIFS network share backend (auto-mount).
*/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/config"
)

// SMBBackend auto-mounts a CIFS share and delegates to a LocalBackend.
type SMBBackend struct {
	cfg        *config.Config
	mountPoint string
	tempMount  bool // true when mountPoint was created by us
	local      *LocalBackend
}

// NewSMBBackend creates an SMBBackend from config.
func NewSMBBackend(cfg *config.Config) (*SMBBackend, error) {
	return &SMBBackend{cfg: cfg}, nil
}

// Connect mounts the SMB share and initialises the inner LocalBackend.
func (b *SMBBackend) Connect() error {
	r := b.cfg.Remote

	mountPoint := r.MountPoint
	if mountPoint == "" {
		mp, err := os.MkdirTemp("", "continuity-smb-*")
		if err != nil {
			return fmt.Errorf("smb: failed to create mount point: %w", err)
		}
		mountPoint = mp
		b.tempMount = true
	} else {
		if err := os.MkdirAll(mountPoint, 0750); err != nil {
			return fmt.Errorf("smb: failed to create mount point %s: %w", mountPoint, err)
		}
	}

	share := r.ShareName
	if share == "" {
		share = filepath.Base(r.Path)
	}
	source := fmt.Sprintf("//%s/%s", r.Host, share)

	opts := fmt.Sprintf("username=%s,password=%s", r.User, r.Password)
	if r.Port != 0 && r.Port != 445 {
		opts += fmt.Sprintf(",port=%d", r.Port)
	}

	cmd := exec.Command("mount", "-t", "cifs", source, mountPoint, "-o", opts)
	if out, err := cmd.CombinedOutput(); err != nil {
		if b.tempMount {
			os.RemoveAll(mountPoint)
		}
		return fmt.Errorf("smb: mount failed: %w\n%s", err, out)
	}

	b.mountPoint = mountPoint

	// Build a LocalBackend rooted at the remote path within the mount
	subPath := filepath.Join(mountPoint, r.Path)
	if err := os.MkdirAll(subPath, 0755); err != nil {
		b.unmount()
		return fmt.Errorf("smb: failed to create repo dir on share: %w", err)
	}

	b.local = &LocalBackend{basePath: subPath}
	return nil
}

// Close unmounts the share and removes a temp mount point if applicable.
func (b *SMBBackend) Close() error {
	return b.unmount()
}

func (b *SMBBackend) unmount() error {
	if b.mountPoint == "" {
		return nil
	}
	cmd := exec.Command("umount", b.mountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("smb: umount failed: %w\n%s", err, out)
	}
	if b.tempMount {
		os.RemoveAll(b.mountPoint)
	}
	b.mountPoint = ""
	return nil
}

func (b *SMBBackend) ReadFile(path string) ([]byte, error)                       { return b.local.ReadFile(path) }
func (b *SMBBackend) WriteFile(p string, d []byte, m os.FileMode) error          { return b.local.WriteFile(p, d, m) }
func (b *SMBBackend) MkdirAll(path string, perm os.FileMode) error               { return b.local.MkdirAll(path, perm) }
func (b *SMBBackend) Remove(path string) error                                   { return b.local.Remove(path) }
func (b *SMBBackend) RemoveAll(path string) error                                { return b.local.RemoveAll(path) }
func (b *SMBBackend) ReadDir(path string) ([]os.DirEntry, error)                 { return b.local.ReadDir(path) }
func (b *SMBBackend) Stat(path string) (os.FileInfo, error)                      { return b.local.Stat(path) }
func (b *SMBBackend) Walk(root string, fn filepath.WalkFunc) error               { return b.local.Walk(root, fn) }
func (b *SMBBackend) Rename(oldPath, newPath string) error                       { return b.local.Rename(oldPath, newPath) }
func (b *SMBBackend) CopyFromNative(nativeSrc, backendDst string) error          { return b.local.CopyFromNative(nativeSrc, backendDst) }
func (b *SMBBackend) CopyToNative(backendSrc, nativeDst string) error            { return b.local.CopyToNative(backendSrc, nativeDst) }
func (b *SMBBackend) BasePath() string                                           { return b.local.BasePath() }
func (b *SMBBackend) IsLocal() bool                                              { return true }
func (b *SMBBackend) SupportsDeduplicate() bool                                  { return true }
