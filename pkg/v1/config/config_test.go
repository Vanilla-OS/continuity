package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.RepositoryPath != DefaultRepositoryPath {
		t.Errorf("Expected RepositoryPath=%s, got %s", DefaultRepositoryPath, cfg.RepositoryPath)
	}

	if cfg.MaxParallelWorkers != 2 {
		t.Errorf("Expected MaxParallelWorkers=2, got %d", cfg.MaxParallelWorkers)
	}

	if len(cfg.ExcludePatterns) == 0 {
		t.Error("Expected non-empty ExcludePatterns")
	}
}

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	confDir := filepath.Join(tmpDir, "continuity")
	os.MkdirAll(confDir, 0755)

	cfgPath := filepath.Join(confDir, "config.json")
	testCfg := map[string]interface{}{
		"repository_path":      "/custom/path",
		"retention_keep_last":  10,
		"max_parallel_workers": 4,
	}

	data, _ := json.MarshalIndent(testCfg, "", "  ")
	os.WriteFile(cfgPath, data, 0644)

	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Unsetenv("XDG_CONFIG_HOME")

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.RepositoryPath != "/custom/path" {
		t.Errorf("Expected RepositoryPath=/custom/path, got %s", loaded.RepositoryPath)
	}

	if loaded.RetentionKeepLast != 10 {
		t.Errorf("Expected RetentionKeepLast=10, got %d", loaded.RetentionKeepLast)
	}
}

func TestLoadFallbackToDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail: %v", err)
	}

	if cfg.RepositoryPath != DefaultRepositoryPath {
		t.Errorf("Expected default RepositoryPath, got %s", cfg.RepositoryPath)
	}
}
