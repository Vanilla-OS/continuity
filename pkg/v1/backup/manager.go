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
	sdkbackup "github.com/vanilla-os/sdk/pkg/v1/backup"
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
func NewManager(app *app.App, repoMgr *repo.Manager, excludePatterns []string, enabledProviders []string, dryRun bool) *Manager {
	allProviders := map[string]providers.BackupProvider{
		"userdata": providers.NewUserDataProvider(excludePatterns),
		"flatpak":  providers.NewFlatpakProvider(),
		"abroot":   providers.NewABRootProvider(),
	}

	var activeProviders []providers.BackupProvider
	for _, name := range enabledProviders {
		if provider, ok := allProviders[name]; ok {
			activeProviders = append(activeProviders, provider)
		}
	}

	return &Manager{
		App:       app,
		RepoMgr:   repoMgr,
		DryRun:    dryRun,
		Providers: activeProviders,
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
func (m *Manager) ListBackups(detailed bool) error {
	snapshots, err := m.RepoMgr.ListSnapshots()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	m.App.Log.Term.Info().Msgf("Found %d backups:", len(snapshots))

	if detailed {
		for _, snapshot := range snapshots {
			m.App.Log.Term.Info().Msgf("  ID:        %s", snapshot.ID)
			m.App.Log.Term.Info().Msgf("  Created:   %s", snapshot.CreatedAt.Format(time.RFC3339))
			m.App.Log.Term.Info().Msgf("  Dedup:     %v", snapshot.Deduplicate)

			size, err := m.RepoMgr.GetSnapshotSize(snapshot.ID)
			if err == nil {
				m.App.Log.Term.Info().Msgf("  Size:      %s", formatBytes(size))
			}

			providers, err := m.RepoMgr.GetSnapshotProviders(snapshot.ID)
			if err == nil && len(providers) > 0 {
				m.App.Log.Term.Info().Msgf("  Providers: %v", providers)
			}

			m.App.Log.Term.Info().Msg("")
		}
	} else {
		for _, snapshot := range snapshots {
			m.App.Log.Term.Info().Msgf("  - %s (%s)", snapshot.ID, snapshot.CreatedAt.Format(time.RFC3339))
		}
	}

	return nil
}

// InspectBackup shows detailed information about a specific backup
func (m *Manager) InspectBackup(snapshotID string) error {
	snapshots, err := m.RepoMgr.ListSnapshots()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	var target *sdkbackup.SnapshotManifest
	for i := range snapshots {
		if snapshots[i].ID == snapshotID {
			target = &snapshots[i]
			break
		}
	}

	if target == nil {
		return fmt.Errorf("snapshot not found: %s", snapshotID)
	}

	m.App.Log.Term.Info().Msgf("===== SNAPSHOT: %s =====", target.ID)
	m.App.Log.Term.Info().Msgf("Created:      %s", target.CreatedAt.Format(time.RFC3339))
	m.App.Log.Term.Info().Msgf("Source:       %s", target.SourcePath)
	m.App.Log.Term.Info().Msgf("Deduplication: %v", target.Deduplicate)

	size, err := m.RepoMgr.GetSnapshotSize(target.ID)
	if err == nil {
		m.App.Log.Term.Info().Msgf("Size:         %s", formatBytes(size))
	}

	m.App.Log.Term.Info().Msg("\nProviders included:")
	providers, err := m.RepoMgr.GetSnapshotProviders(target.ID)
	if err == nil {
		for _, provider := range providers {
			m.App.Log.Term.Info().Msgf("  - %s", provider)
		}

		// Show provider content
		m.App.Log.Term.Info().Msg("\nProvider content:")
		for _, provider := range providers {
			content, err := m.RepoMgr.GetProviderContent(target.ID, provider)
			if err != nil {
				m.App.Log.Term.Warn().Msgf("  %s: Could not read content: %v", provider, err)
				continue
			}

			if len(content) > 0 {
				m.App.Log.Term.Info().Msgf("\n  %s:", provider)
				for _, line := range content {
					m.App.Log.Term.Info().Msgf("    %s", line)
				}
			}
		}
	} else {
		m.App.Log.Term.Warn().Msgf("  Could not determine providers: %v", err)
	}

	return nil
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

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
