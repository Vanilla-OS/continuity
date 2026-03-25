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

	"github.com/vanilla-os/continuity/pkg/v1/config"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/backup"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
)

// Manager wraps SDK backup.Repository
type Manager struct {
	App    *app.App
	Config *config.Config
	Repo   *backup.Repository
}

// NewManager creates or opens a backup repository
func NewManager(app *app.App, cfg *config.Config) (*Manager, error) {
	repo, err := backup.OpenRepository(cfg.RepositoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	return &Manager{
		App:    app,
		Config: cfg,
		Repo:   repo,
	}, nil
}

// CreateSnapshot creates a new backup snapshot
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

	// Auto-prune if retention policy is set
	if m.Config.RetentionKeepLast > 0 {
		m.App.Log.Term.Info().Msgf("Applying retention policy (keep last %d)", m.Config.RetentionKeepLast)
		if err := m.PruneOld(m.Config.RetentionKeepLast); err != nil {
			m.App.Log.Term.Warn().Msgf("Retention pruning failed (non-critical): %v", err)
		}
	}

	return snapshot.Manifest.ID, nil
}

// ListSnapshots returns all snapshots in the repository
func (m *Manager) ListSnapshots() ([]backup.SnapshotManifest, error) {
	snapshots, err := m.Repo.ListSnapshots()
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	return snapshots, nil
}

// RestoreSnapshot restores a snapshot to the specified destination
func (m *Manager) RestoreSnapshot(snapshotID, destination string) error {
	m.App.Log.Term.Info().Msgf("Restoring snapshot %s to %s", snapshotID, destination)

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

// PruneOld removes old snapshots keeping only the most recent keepLast
func (m *Manager) PruneOld(keepLast int) error {
	m.App.Log.Term.Info().Msgf("Pruning old snapshots (keeping last %d)", keepLast)

	deleted, err := m.Repo.PruneKeepLast(keepLast)
	if err != nil {
		return fmt.Errorf("failed to prune snapshots: %w", err)
	}

	m.App.Log.Term.Info().Msgf("Pruned %d old snapshots", len(deleted))
	return nil
}

// GetSnapshotSize calculates the total size of a snapshot
func (m *Manager) GetSnapshotSize(snapshotID string) (int64, error) {
	snapshotPath := fmt.Sprintf("%s/snapshots/%s", m.Config.RepositoryPath, snapshotID)
	size, err := calculateDirSize(snapshotPath)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate size: %w", err)
	}
	return size, nil
}

// GetSnapshotProviders returns the list of providers used in a snapshot
func (m *Manager) GetSnapshotProviders(snapshotID string) ([]string, error) {
	snapshotPath := fmt.Sprintf("%s/snapshots/%s/tree", m.Config.RepositoryPath, snapshotID)

	var providers []string

	// Check for provider-specific directories
	if exists(fmt.Sprintf("%s/UserData", snapshotPath)) {
		providers = append(providers, "UserData")
	}
	if exists(fmt.Sprintf("%s/Flatpak", snapshotPath)) {
		providers = append(providers, "Flatpak")
	}
	if exists(fmt.Sprintf("%s/ABRoot", snapshotPath)) {
		providers = append(providers, "ABRoot")
	}

	return providers, nil
}

// GetProviderContent returns human-readable content summary for a provider
func (m *Manager) GetProviderContent(snapshotID, providerName string) ([]string, error) {
	snapshotPath := fmt.Sprintf("%s/snapshots/%s/tree", m.Config.RepositoryPath, snapshotID)
	providerPath := fmt.Sprintf("%s/%s", snapshotPath, providerName)

	var content []string

	switch providerName {
	case "Flatpak":
		// Read flatpak.json
		jsonPath := fmt.Sprintf("%s/flatpak.json", providerPath)
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read flatpak.json: %w", err)
		}

		var apps []map[string]interface{}
		if err := json.Unmarshal(data, &apps); err != nil {
			return nil, fmt.Errorf("failed to parse flatpak.json: %w", err)
		}

		for _, app := range apps {
			name := app["name"]
			appID := app["application"]
			content = append(content, fmt.Sprintf("%s (%s)", name, appID))
		}

	case "ABRoot":
		// List files in abroot directory
		err := filepath.Walk(providerPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(providerPath, path)
				content = append(content, fmt.Sprintf("%s (%s)", relPath, formatBytes(info.Size())))
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk ABRoot directory: %w", err)
		}

	case "UserData":
		// List top-level directories (user homes)
		entries, err := os.ReadDir(providerPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read UserData directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				dirPath := fmt.Sprintf("%s/%s", providerPath, entry.Name())
				size, _ := calculateDirSize(dirPath)
				content = append(content, fmt.Sprintf("%s (%s)", entry.Name(), formatBytes(size)))
			}
		}
	}

	return content, nil
}

func calculateDirSize(path string) (int64, error) {
var size int64
err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
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

func exists(path string) bool {
_, err := os.Stat(path)
return err == nil
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
