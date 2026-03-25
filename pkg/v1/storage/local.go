package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Local filesystem backend.
*/

import (
	"os"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/config"
	sdkfs "github.com/vanilla-os/sdk/pkg/v1/fs"
)

// LocalBackend wraps the local filesystem via os.* and filepath.*.
type LocalBackend struct {
	basePath string
}

// NewLocalBackend creates a LocalBackend rooted at cfg.RepositoryPath.
func NewLocalBackend(cfg *config.Config) *LocalBackend {
	return &LocalBackend{basePath: cfg.RepositoryPath}
}

func (b *LocalBackend) Connect() error { return nil }
func (b *LocalBackend) Close() error   { return nil }

func (b *LocalBackend) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (b *LocalBackend) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (b *LocalBackend) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (b *LocalBackend) Remove(path string) error {
	return os.Remove(path)
}

func (b *LocalBackend) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (b *LocalBackend) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (b *LocalBackend) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (b *LocalBackend) Walk(root string, fn filepath.WalkFunc) error {
	return filepath.Walk(root, fn)
}

func (b *LocalBackend) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (b *LocalBackend) CopyFromNative(nativeSrc, backendDst string) error {
	return sdkfs.CopyTree(nativeSrc, backendDst, sdkfs.CopyTreeOptions{
		Workers:             2,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	})
}

func (b *LocalBackend) CopyToNative(backendSrc, nativeDst string) error {
	return sdkfs.CopyTree(backendSrc, nativeDst, sdkfs.CopyTreeOptions{
		Workers:             2,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	})
}

func (b *LocalBackend) BasePath() string         { return b.basePath }
func (b *LocalBackend) IsLocal() bool            { return true }
func (b *LocalBackend) SupportsDeduplicate() bool { return true }
