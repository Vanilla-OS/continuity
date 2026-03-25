package backup

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Backup manager orchestrates backup providers.
*/

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vanilla-os/continuity/pkg/v1/providers"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
)

// Manager orchestrates backups using providers
type Manager struct {
	App       *app.App
	RepoMgr   *repo.Manager
	Providers []providers.BackupProvider
	DryRun    bool
}

// NewManager creates a new backup manager
func NewManager(app *app.App, repoMgr *repo.Manager, excludePatterns []string, dryRun bool) *Manager {
	return &Manager{
		App:     app,
		RepoMgr: repoMgr,
		DryRun:  dryRun,
		Providers: []providers.BackupProvider{
			providers.NewUserDataProvider(excludePatterns),
			providers.NewFlatpakProvider(),
			providers.NewABRootProvider(),
		},
	}
}

// RunBackup executes a full system backup
func (m *Manager) RunBackup(label string) (string, error) {
	if m.DryRun {
		m.App.Log.Term.Warn().Msg("===== DRY-RUN MODE: NO CHANGES WILL BE MADE =====")
	}
	m.App.Log.Term.Info().Msg("===== STARTING FULL SYSTEM BACKUP =====")

	stagingDir, err := os.MkdirTemp("", "continuity-backup-*")
	if err != nil {
		return "", fmt.Errorf("failed to create staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	for _, provider := range m.Providers {
		m.App.Log.Term.Info().Msgf("Running provider: %s", provider.Name())

		if m.DryRun {
			m.App.Log.Term.Info().Msgf("[DRY-RUN] Would backup provider: %s", provider.Name())
			continue
		}

		providerPath, err := provider.Backup(m.App)
		if err != nil {
			m.App.Log.Term.Error().Msgf("Provider %s failed: %v", provider.Name(), err)
			continue
		}
		defer os.RemoveAll(providerPath)

		destPath := filepath.Join(stagingDir, provider.Name())
		copyOpts := fs.CopyTreeOptions{
			Workers:             2,
			PreserveOwnership:   true,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		}

		if err := fs.CopyTree(providerPath, destPath, copyOpts); err != nil {
			m.App.Log.Term.Error().Msgf("Failed to stage %s: %v", provider.Name(), err)
			continue
		}

		m.App.Log.Term.Info().Msgf("Provider %s completed", provider.Name())
	}

	if m.DryRun {
		m.App.Log.Term.Warn().Msg("[DRY-RUN] Snapshot creation skipped")
		m.App.Log.Term.Info().Msg("===== DRY-RUN COMPLETE (NO CHANGES MADE) =====")
		return "dry-run-snapshot-id", nil
	}

	tags := map[string]string{
		"type":      "full",
		"label":     label,
		"timestamp": time.Now().Format(time.RFC3339),
		"hostname":  getHostname(),
	}

	snapshotID, err := m.RepoMgr.CreateSnapshot(stagingDir, tags)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	m.App.Log.Term.Info().Msgf("===== BACKUP COMPLETE: %s =====", snapshotID)
	return snapshotID, nil
}

// ListBackups lists all available backups
func (m *Manager) ListBackups() error {
	snapshots, err := m.RepoMgr.ListSnapshots()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	m.App.Log.Term.Info().Msgf("Found %d backups:", len(snapshots))
	for _, snapshot := range snapshots {
		m.App.Log.Term.Info().Msgf("  - %s (%s)", snapshot.ID, snapshot.CreatedAt.Format(time.RFC3339))
	}

	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
