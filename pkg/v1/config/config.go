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
	RepositoryPath     string   `json:"repository_path"`
	DefaultDeduplicate bool     `json:"default_deduplicate"`
	MaxParallelWorkers int      `json:"max_parallel_workers"`
	RetentionKeepLast  int      `json:"retention_keep_last"`
	ExcludePatterns    []string `json:"exclude_patterns"`
	EnabledProviders   []string `json:"enabled_providers"`
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
