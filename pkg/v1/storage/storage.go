package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Pluggable storage backend interface and factory.
*/

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/config"
)

// Backend abstracts all filesystem operations for a backup destination.
// Implementations must write directly to the backend without local staging.
type Backend interface {
	Connect() error
	Close() error

	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	Walk(root string, fn filepath.WalkFunc) error
	Rename(oldPath, newPath string) error

	// CopyFromNative streams a local filesystem path directly to this backend.
	// For SFTP/FTP this uploads without local staging.
	CopyFromNative(nativeSrc, backendDst string) error

	// CopyToNative downloads from this backend to a local filesystem path.
	CopyToNative(backendSrc, nativeDst string) error

	// BasePath is the root repository path on this backend.
	BasePath() string

	// IsLocal returns true when the backend exposes a real local filesystem path.
	// true → SDK Repository can be used (local, smb, nfs).
	// false → SDK is bypassed, snapshot logic is handled internally (sftp, ftp).
	IsLocal() bool

	// SupportsDeduplicate returns true when content deduplication is available.
	// Only true for IsLocal() backends backed by dabadee.
	SupportsDeduplicate() bool
}

// NewBackend creates the appropriate backend from config.
// If cfg.Remote is nil or type is "local", returns a LocalBackend.
func NewBackend(cfg *config.Config) (Backend, error) {
	if cfg.Remote == nil || cfg.Remote.Type == "" || cfg.Remote.Type == "local" {
		return NewLocalBackend(cfg), nil
	}
	if err := validateRemoteConfig(cfg.Remote); err != nil {
		return nil, err
	}
	switch cfg.Remote.Type {
	case "sftp":
		return NewSFTPBackend(cfg), nil
	case "ftp":
		return NewFTPBackend(cfg), nil
	case "smb":
		return NewSMBBackend(cfg)
	case "nfs":
		return NewNFSBackend(cfg)
	default:
		return NewLocalBackend(cfg), nil
	}
}

// validateRemoteConfig checks that the remote config is sane before connecting.
func validateRemoteConfig(r *config.RemoteConfig) error {
	if r.Host == "" {
		return fmt.Errorf("remote config: host is required for backend type %q", r.Type)
	}
	if r.Path == "" || r.Path == "/" {
		return fmt.Errorf("remote config: path %q is not valid — set a dedicated directory (e.g. /backups/continuity)", r.Path)
	}
	switch r.Type {
	case "sftp", "ftp":
		if r.User == "" {
			return fmt.Errorf("remote config: user is required for %s backend", r.Type)
		}
		if r.Password == "" && r.KeyFile == "" {
			return fmt.Errorf("remote config: either password or key_file is required for %s backend", r.Type)
		}
	case "smb":
		if r.ShareName == "" {
			return fmt.Errorf("remote config: share_name is required for smb backend")
		}
	}
	return nil
}
