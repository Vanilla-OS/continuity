package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Local filesystem backend.
*/

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/config"
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

func (b *LocalBackend) CopyFromNative(nativeSrc, backendDst string, excludePatterns []string) error {
	resolvedSrc, err := filepath.EvalSymlinks(nativeSrc)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", nativeSrc, err)
	}

	return filepath.Walk(resolvedSrc, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r\033[K  ⚠ skipped %s: %v\n", localPath, err)
			return nil
		}

		relPath, err := filepath.Rel(resolvedSrc, localPath)
		if err != nil {
			return err
		}

		if relPath != "." && shouldExclude(relPath, excludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dstPath := filepath.Join(backendDst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		src, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

func (b *LocalBackend) CopyToNative(backendSrc, nativeDst string) error {
	return filepath.Walk(backendSrc, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(backendSrc, srcPath)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(nativeDst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		src, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

func (b *LocalBackend) BasePath() string         { return b.basePath }
func (b *LocalBackend) IsLocal() bool            { return true }
func (b *LocalBackend) SupportsDeduplicate() bool { return true }
