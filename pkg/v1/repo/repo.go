package repo

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Repository manager wrapper around SDK backup primitives.
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vanilla-os/continuity/pkg/v1/config"
	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/backup"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
)

// Manager wraps SDK backup.Repository
type Manager struct {
	App     *app.App
	Config  *config.Config
	Repo    *backup.Repository
	Backend storage.Backend
}

// NewManager creates or opens a backup repository.
// For local (IsLocal) backends the SDK Repository is used for dedup support.
// For remote backends (sftp, ftp) the SDK is bypassed and Repo is nil.
func NewManager(app *app.App, cfg *config.Config, backend storage.Backend) (*Manager, error) {
	m := &Manager{
		App:     app,
		Config:  cfg,
		Backend: backend,
	}

	if backend.IsLocal() {
		repo, err := backup.OpenRepository(backend.BasePath())
		if err != nil {
			return nil, fmt.Errorf("failed to open repository: %w", err)
		}
		m.Repo = repo
	}

	return m, nil
}

// CreateSnapshot creates a new backup snapshot.
// Only called for IsLocal() backends; delegates to the SDK.
func (m *Manager) CreateSnapshot(source string, _ map[string]string) (string, error) {
	m.App.Log.Term.Info().Msgf("Creating snapshot from %s", source)

	opts := backup.CreateSnapshotOptions{
		Deduplicate: m.Config.DefaultDeduplicate,
		CopyOptions: fs.CopyTreeOptions{
			Workers:             m.Config.MaxParallelWorkers,
			PreserveOwnership:   true,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		},
	}

	snapshot, err := m.Repo.CreateSnapshot(source, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	m.App.Log.Term.Info().Msgf("Snapshot created: %s", snapshot.Manifest.ID)

	if m.Config.RetentionKeepLast > 0 {
		m.App.Log.Term.Info().Msgf("Applying retention policy (keep last %d)", m.Config.RetentionKeepLast)
		if err := m.PruneOld(m.Config.RetentionKeepLast); err != nil {
			m.App.Log.Term.Warn().Msgf("Retention pruning failed (non-critical): %v", err)
		}
	}

	return snapshot.Manifest.ID, nil
}

// ListSnapshots returns all snapshots in the repository.
func (m *Manager) ListSnapshots() ([]backup.SnapshotManifest, error) {
	if m.Repo != nil {
		snapshots, err := m.Repo.ListSnapshots()
		if err != nil {
			return nil, fmt.Errorf("failed to list snapshots: %w", err)
		}
		return snapshots, nil
	}

	snapshotsPath := filepath.Join(m.Backend.BasePath(), "snapshots")
	entries, err := m.Backend.ReadDir(snapshotsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var manifests []backup.SnapshotManifest
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".tmp" {
			continue
		}
		manifestPath := filepath.Join(snapshotsPath, entry.Name(), "manifest.json")
		data, err := m.Backend.ReadFile(manifestPath)
		if err != nil {
			m.App.Log.Term.Warn().Msgf("Skipping snapshot %s: %v", entry.Name(), err)
			continue
		}
		var manifest backup.SnapshotManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			m.App.Log.Term.Warn().Msgf("Skipping snapshot %s (bad manifest): %v", entry.Name(), err)
			continue
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// RestoreSnapshot restores a snapshot to the specified destination.
func (m *Manager) RestoreSnapshot(snapshotID, destination string) error {
	m.App.Log.Term.Info().Msgf("Restoring snapshot %s to %s", snapshotID, destination)

	if m.Repo != nil {
		copyOpts := fs.CopyTreeOptions{
			Workers:             m.Config.MaxParallelWorkers,
			PreserveOwnership:   true,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		}
		if err := m.Repo.RestoreSnapshot(snapshotID, destination, copyOpts); err != nil {
			return fmt.Errorf("failed to restore snapshot: %w", err)
		}
		m.App.Log.Term.Info().Msg("Restore completed successfully")
		return nil
	}

	treePath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID, "tree")
	if err := m.Backend.CopyToNative(treePath, destination); err != nil {
		return fmt.Errorf("failed to restore snapshot from remote: %w", err)
	}

	m.App.Log.Term.Info().Msg("Restore completed successfully")
	return nil
}

// PruneOld removes old snapshots keeping only the most recent keepLast.
func (m *Manager) PruneOld(keepLast int) error {
	m.App.Log.Term.Info().Msgf("Pruning old snapshots (keeping last %d)", keepLast)

	if m.Repo != nil {
		deleted, err := m.Repo.PruneKeepLast(keepLast)
		if err != nil {
			return fmt.Errorf("failed to prune snapshots: %w", err)
		}
		m.App.Log.Term.Info().Msgf("Pruned %d old snapshots", len(deleted))
		return nil
	}

	snapshots, err := m.ListSnapshots()
	if err != nil {
		return fmt.Errorf("failed to list snapshots for pruning: %w", err)
	}

	if len(snapshots) <= keepLast {
		return nil
	}

	// Sort oldest first
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.Before(snapshots[j].CreatedAt)
	})

	toDelete := snapshots[:len(snapshots)-keepLast]
	snapshotsPath := filepath.Join(m.Backend.BasePath(), "snapshots")
	for _, snap := range toDelete {
		snapPath := filepath.Join(snapshotsPath, snap.ID)
		if err := m.Backend.RemoveAll(snapPath); err != nil {
			m.App.Log.Term.Warn().Msgf("Failed to delete snapshot %s: %v", snap.ID, err)
		} else {
			m.App.Log.Term.Info().Msgf("Deleted snapshot %s", snap.ID)
		}
	}

	m.App.Log.Term.Info().Msgf("Pruned %d old snapshots", len(toDelete))
	return nil
}

// GetSnapshotSize calculates the total size of a snapshot.
func (m *Manager) GetSnapshotSize(snapshotID string) (int64, error) {
	snapshotPath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID)
	var size int64
	err := m.Backend.Walk(snapshotPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to calculate size: %w", err)
	}
	return size, nil
}

// GetSnapshotProviders returns the list of providers used in a snapshot.
func (m *Manager) GetSnapshotProviders(snapshotID string) ([]string, error) {
	snapshotPath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID, "tree")

	var providers []string
	for _, name := range []string{"UserData", "Flatpak", "ABRoot"} {
		if _, err := m.Backend.Stat(filepath.Join(snapshotPath, name)); err == nil {
			providers = append(providers, name)
		}
	}
	return providers, nil
}

// ProviderContent holds structured table data for a provider's content.
type ProviderContent struct {
	Headers []string
	Rows    [][]string
}

// GetProviderContent returns structured table data for a provider's content.
func (m *Manager) GetProviderContent(snapshotID, providerName string) (*ProviderContent, error) {
	snapshotPath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID, "tree")
	providerPath := filepath.Join(snapshotPath, providerName)

	result := &ProviderContent{}

	switch providerName {
	case "Flatpak":
		result.Headers = []string{"Name", "ID"}

		jsonPath := filepath.Join(providerPath, "flatpak-apps.json")
		data, err := m.Backend.ReadFile(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read flatpak-apps.json: %w", err)
		}

		var apps []map[string]interface{}
		if err := json.Unmarshal(data, &apps); err != nil {
			return nil, fmt.Errorf("failed to parse flatpak-apps.json: %w", err)
		}

		for _, app := range apps {
			name := fmt.Sprintf("%v", app["name"])
			appID := fmt.Sprintf("%v", app["ref"])
			result.Rows = append(result.Rows, []string{name, appID})
		}

	case "ABRoot":
		result.Headers = []string{"File", "Size"}

		err := m.Backend.Walk(providerPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(providerPath, path)
				result.Rows = append(result.Rows, []string{relPath, formatBytes(info.Size())})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk ABRoot directory: %w", err)
		}

	case "UserData":
		result.Headers = []string{"User", "Size"}

		entries, err := m.Backend.ReadDir(filepath.Join(providerPath, "home"))
		if err != nil {
			// Fall back to reading providerPath directly if home subdir not present
			entries, err = m.Backend.ReadDir(providerPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read UserData directory: %w", err)
			}
		}

		for _, entry := range entries {
			if entry.IsDir() {
				dirPath := filepath.Join(providerPath, "home", entry.Name())
				size, _ := m.dirSize(dirPath)
				result.Rows = append(result.Rows, []string{entry.Name(), formatBytes(size)})
			}
		}
	}

	return result, nil
}

func (m *Manager) dirSize(path string) (int64, error) {
	var size int64
	err := m.Backend.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
