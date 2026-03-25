package config

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Configuration management for Vanilla Continuity.
*/

import (
	"github.com/vanilla-os/sdk/pkg/v1/conf"
)

const (
	// DefaultRepositoryPath is the default backup repository location
	DefaultRepositoryPath = "/var/lib/vanilla-continuity/repo"
)

// Config represents Vanilla Continuity configuration
type Config struct {
	RepositoryPath     string        `json:"repository_path"`
	DefaultDeduplicate bool          `json:"default_deduplicate"`
	MaxParallelWorkers int           `json:"max_parallel_workers"`
	RetentionKeepLast  int           `json:"retention_keep_last"`
	ExcludePatterns    []string      `json:"exclude_patterns"`
	EnabledProviders   []string      `json:"enabled_providers"`
	Remote             *RemoteConfig `json:"remote,omitempty"`
}

// RemoteConfig holds configuration for a remote storage backend.
// When set, backups are stored on the remote instead of RepositoryPath.
type RemoteConfig struct {
	// Type is the backend type: local, sftp, ftp, smb, nfs
	Type string `json:"type"`
	// Host is the remote hostname or IP address
	Host string `json:"host"`
	// Port is the remote port (defaults depend on type: 22 for sftp, 21 for ftp, 445 for smb, 2049 for nfs)
	Port int `json:"port"`
	// User is the login username
	User string `json:"user"`
	// Password is the login password (optional if using KeyFile)
	Password string `json:"password,omitempty"`
	// KeyFile is the path to an SSH private key (sftp only)
	KeyFile string `json:"key_file,omitempty"`
	// Path is the remote directory path where backups are stored
	Path string `json:"path"`
	// MountPoint is the local directory used as mount point for smb/nfs backends.
	// Defaults to a temp directory if not set.
	MountPoint string `json:"mount_point,omitempty"`
	// ShareName is the SMB share name (smb only)
	ShareName string `json:"share_name,omitempty"`
}

// Default returns a default configuration
func Default() *Config {
	return &Config{
		RepositoryPath:     DefaultRepositoryPath,
		DefaultDeduplicate: false,
		MaxParallelWorkers: 2,
		RetentionKeepLast:  7,
		ExcludePatterns: []string{
			".cache",
			".local/share/Trash",
			"node_modules",
			".tmp",
			"*.tmp",
		},
		EnabledProviders: []string{
			"userdata",
			"flatpak",
			"abroot",
		},
	}
}

// Load reads configuration using SDK standard paths
func Load() (*Config, error) {
	cfg, err := conf.NewBuilder[Config]("continuity").
		WithCascading(true).
		WithOptional(true).
		Build()

	if err != nil {
		return Default(), nil
	}

	// Apply defaults for unset fields
	if cfg.RepositoryPath == "" {
		cfg.RepositoryPath = DefaultRepositoryPath
	}
	if cfg.MaxParallelWorkers == 0 {
		cfg.MaxParallelWorkers = 2
	}
	if cfg.RetentionKeepLast == 0 {
		cfg.RetentionKeepLast = 7
	}
	if len(cfg.ExcludePatterns) == 0 {
		cfg.ExcludePatterns = []string{
			".cache",
			".local/share/Trash",
			"node_modules",
			".tmp",
			"*.tmp",
		}
	}
	if len(cfg.EnabledProviders) == 0 {
		cfg.EnabledProviders = []string{
			"userdata",
			"flatpak",
			"abroot",
		}
	}

	return cfg, nil
}
