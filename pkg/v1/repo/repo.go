package repo

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Repository manager wrapper around SDK backup primitives.
*/

import (
	"fmt"

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
func (m *Manager) CreateSnapshot(source string, tags map[string]string) (string, error) {
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
