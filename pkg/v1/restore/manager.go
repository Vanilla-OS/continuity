package restore

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Restore manager orchestrates restore operations.
*/

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/providers"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
)

// Manager orchestrates restore operations
type Manager struct {
	App       *app.App
	RepoMgr   *repo.Manager
	Backend   storage.Backend
	Providers map[string]providers.BackupProvider
	DryRun    bool
}

// NewManager creates a new restore manager
func NewManager(app *app.App, repoMgr *repo.Manager, backend storage.Backend, enabledProviders []string, dryRun bool) *Manager {
	allProviders := map[string]providers.BackupProvider{
		"userdata": providers.NewUserDataProvider(nil),
		"flatpak":  providers.NewFlatpakProvider(),
		"abroot":   providers.NewABRootProvider(),
	}

	activeProviders := make(map[string]providers.BackupProvider)
	for _, name := range enabledProviders {
		providerKey := name
		switch name {
		case "userdata":
			providerKey = "UserData"
		case "flatpak":
			providerKey = "Flatpak"
		case "abroot":
			providerKey = "ABRoot"
		}

		if provider, ok := allProviders[name]; ok {
			activeProviders[providerKey] = provider
		}
	}

	return &Manager{
		App:       app,
		RepoMgr:   repoMgr,
		Backend:   backend,
		DryRun:    dryRun,
		Providers: activeProviders,
	}
}

// RunRestore executes a full system restore
func (m *Manager) RunRestore(snapshotID string) error {
	if m.DryRun {
		m.App.Log.Term.Warn().Msg("===== DRY-RUN MODE: NO CHANGES WILL BE MADE =====")
	}
	m.App.Log.Term.Info().Msgf("===== STARTING RESTORE FROM %s =====", snapshotID)

	if m.Backend.IsLocal() {
		return m.runLocalRestore(snapshotID)
	}
	return m.runRemoteRestore(snapshotID)
}

// runLocalRestore extracts the snapshot to a local staging dir, then restores per-provider.
func (m *Manager) runLocalRestore(snapshotID string) error {
	stagingDir, err := os.MkdirTemp("", "continuity-restore-*")
	if err != nil {
		return fmt.Errorf("failed to create staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	if m.DryRun {
		m.App.Log.Term.Info().Msg("[DRY-RUN] Would restore snapshot to staging")
	} else {
		if err := m.RepoMgr.RestoreSnapshot(snapshotID, stagingDir); err != nil {
			return fmt.Errorf("failed to restore snapshot: %w", err)
		}
	}

	return m.restoreProviders(snapshotID, stagingDir, true)
}

// runRemoteRestore reads providers directly from the remote backend.
func (m *Manager) runRemoteRestore(snapshotID string) error {
	treePath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID, "tree")
	return m.restoreProviders(snapshotID, treePath, false)
}

func (m *Manager) restoreProviders(snapshotID, basePath string, isStaging bool) error {
	abrootRestored := false

	for name, provider := range m.Providers {
		providerPath := filepath.Join(basePath, name)

		if !m.DryRun && isStaging {
			if _, err := os.Stat(providerPath); os.IsNotExist(err) {
				m.App.Log.Term.Warn().Msgf("Provider %s data not found in backup, skipping", name)
				continue
			}
		}

		m.App.Log.Term.Info().Msgf("Restoring provider: %s", name)

		if m.DryRun {
			m.App.Log.Term.Info().Msgf("[DRY-RUN] Would restore provider: %s", name)
			if name == "ABRoot" {
				abrootRestored = true
			}
			continue
		}

		if err := provider.Restore(m.App, m.Backend, providerPath); err != nil {
			m.App.Log.Term.Error().Msgf("Provider %s restore failed: %v", name, err)
			continue
		}

		if name == "ABRoot" {
			abrootRestored = true
		}

		m.App.Log.Term.Info().Msgf("Provider %s restore completed", name)
	}

	if abrootRestored {
		if m.DryRun {
			m.App.Log.Term.Info().Msg("[DRY-RUN] Would run: abroot pkg sync")
		} else {
			m.App.Log.Term.Info().Msg("Triggering ABRoot package sync...")
			cmd := exec.Command("abroot", "pkg", "sync")
			if output, err := cmd.CombinedOutput(); err != nil {
				m.App.Log.Term.Warn().Msgf("ABRoot pkg sync failed (non-critical): %v\n%s", err, string(output))
			} else {
				m.App.Log.Term.Info().Msg("ABRoot package sync completed")
			}
		}
	}

	if m.DryRun {
		m.App.Log.Term.Info().Msgf("===== DRY-RUN COMPLETE (NO CHANGES MADE) =====")
	} else {
		m.App.Log.Term.Info().Msg("===== RESTORE COMPLETE =====")
	}

	return nil
}
