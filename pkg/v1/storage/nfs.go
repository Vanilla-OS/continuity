package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: NFS network filesystem backend (auto-mount).
*/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/config"
)

// NFSBackend auto-mounts an NFS export and delegates to a LocalBackend.
type NFSBackend struct {
	cfg        *config.Config
	mountPoint string
	tempMount  bool
	local      *LocalBackend
}

// NewNFSBackend creates an NFSBackend from config.
func NewNFSBackend(cfg *config.Config) (*NFSBackend, error) {
	return &NFSBackend{cfg: cfg}, nil
}

// Connect mounts the NFS export and initialises the inner LocalBackend.
func (b *NFSBackend) Connect() error {
	r := b.cfg.Remote

	mountPoint := r.MountPoint
	if mountPoint == "" {
		mp, err := os.MkdirTemp("", "continuity-nfs-*")
		if err != nil {
			return fmt.Errorf("nfs: failed to create mount point: %w", err)
		}
		mountPoint = mp
		b.tempMount = true
	} else {
		if err := os.MkdirAll(mountPoint, 0750); err != nil {
			return fmt.Errorf("nfs: failed to create mount point %s: %w", mountPoint, err)
		}
	}

	source := fmt.Sprintf("%s:%s", r.Host, r.Path)
	cmd := exec.Command("mount", "-t", "nfs", source, mountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		if b.tempMount {
			os.RemoveAll(mountPoint)
		}
		return fmt.Errorf("nfs: mount failed: %w\n%s", err, out)
	}

	b.mountPoint = mountPoint

	subPath := mountPoint
	if err := os.MkdirAll(subPath, 0755); err != nil {
		b.unmount()
		return fmt.Errorf("nfs: failed to create repo dir: %w", err)
	}

	b.local = &LocalBackend{basePath: subPath}
	return nil
}

// Close unmounts the NFS share.
func (b *NFSBackend) Close() error {
	return b.unmount()
}

func (b *NFSBackend) unmount() error {
	if b.mountPoint == "" {
		return nil
	}
	cmd := exec.Command("umount", b.mountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nfs: umount failed: %w\n%s", err, out)
	}
	if b.tempMount {
		os.RemoveAll(b.mountPoint)
	}
	b.mountPoint = ""
	return nil
}

func (b *NFSBackend) ReadFile(path string) ([]byte, error)                       { return b.local.ReadFile(path) }
func (b *NFSBackend) WriteFile(p string, d []byte, m os.FileMode) error          { return b.local.WriteFile(p, d, m) }
func (b *NFSBackend) MkdirAll(path string, perm os.FileMode) error               { return b.local.MkdirAll(path, perm) }
func (b *NFSBackend) Remove(path string) error                                   { return b.local.Remove(path) }
func (b *NFSBackend) RemoveAll(path string) error                                { return b.local.RemoveAll(path) }
func (b *NFSBackend) ReadDir(path string) ([]os.DirEntry, error)                 { return b.local.ReadDir(path) }
func (b *NFSBackend) Stat(path string) (os.FileInfo, error)                      { return b.local.Stat(path) }
func (b *NFSBackend) Walk(root string, fn filepath.WalkFunc) error               { return b.local.Walk(root, fn) }
func (b *NFSBackend) Rename(oldPath, newPath string) error                       { return b.local.Rename(oldPath, newPath) }
func (b *NFSBackend) CopyFromNative(nativeSrc, backendDst string, excludePatterns []string) error { return b.local.CopyFromNative(nativeSrc, backendDst, excludePatterns) }
func (b *NFSBackend) CopyToNative(backendSrc, nativeDst string) error            { return b.local.CopyToNative(backendSrc, nativeDst) }
func (b *NFSBackend) BasePath() string                                           { return b.local.BasePath() }
func (b *NFSBackend) IsLocal() bool                                              { return true }
func (b *NFSBackend) SupportsDeduplicate() bool                                  { return true }
