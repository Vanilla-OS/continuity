package backup

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Backup manager orchestrates backup providers.
*/

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vanilla-os/continuity/pkg/v1/providers"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	sdkbackup "github.com/vanilla-os/sdk/pkg/v1/backup"
)

// Manager orchestrates backups using providers
type Manager struct {
	App       *app.App
	RepoMgr   *repo.Manager
	Providers []providers.BackupProvider
	Backend   storage.Backend
	DryRun    bool
}

// NewManager creates a new backup manager
func NewManager(app *app.App, repoMgr *repo.Manager, excludePatterns []string, enabledProviders []string, backend storage.Backend, dryRun bool) *Manager {
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
		Backend:   backend,
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

	if m.Backend.IsLocal() {
		return m.runLocalBackup(label)
	}
	return m.runRemoteBackup(label)
}

// runLocalBackup uses the SDK Repository path with local staging + dedup.
func (m *Manager) runLocalBackup(label string) (string, error) {
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

		destPath := filepath.Join(stagingDir, provider.Name())
		if err := m.Backend.MkdirAll(destPath, 0755); err != nil {
			m.App.Log.Term.Error().Msgf("Failed to create staging dir for %s: %v", provider.Name(), err)
			continue
		}

		if err := provider.Backup(m.App, m.Backend, destPath); err != nil {
			m.App.Log.Term.Error().Msgf("Provider %s failed: %v", provider.Name(), err)
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

// runRemoteBackup writes directly to the remote backend with atomic rename.
func (m *Manager) runRemoteBackup(label string) (string, error) {
	if m.RepoMgr.Config.DefaultDeduplicate {
		backendType := m.RepoMgr.Config.Remote.Type
		m.App.Log.Term.Warn().Msgf(
			"Deduplication is not supported by the %s backend and has been disabled for this run.",
			backendType,
		)
	}

	snapshotID := generateSnapshotID()

	tmpPath := filepath.Join(m.Backend.BasePath(), "snapshots", ".tmp", snapshotID)
	treePath := filepath.Join(tmpPath, "tree")

	if err := m.Backend.MkdirAll(treePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create remote tree dir: %w", err)
	}

	for _, provider := range m.Providers {
		m.App.Log.Term.Info().Msgf("Running provider: %s", provider.Name())

		if m.DryRun {
			m.App.Log.Term.Info().Msgf("[DRY-RUN] Would backup provider: %s", provider.Name())
			continue
		}

		destPath := filepath.Join(treePath, provider.Name())
		if err := m.Backend.MkdirAll(destPath, 0755); err != nil {
			m.App.Log.Term.Error().Msgf("Failed to create remote dir for %s: %v", provider.Name(), err)
			continue
		}

		if err := provider.Backup(m.App, m.Backend, destPath); err != nil {
			m.App.Log.Term.Error().Msgf("Provider %s failed: %v", provider.Name(), err)
			continue
		}

		m.App.Log.Term.Info().Msgf("Provider %s completed", provider.Name())
	}

	if m.DryRun {
		_ = m.Backend.RemoveAll(tmpPath)
		m.App.Log.Term.Warn().Msg("[DRY-RUN] Snapshot creation skipped")
		m.App.Log.Term.Info().Msg("===== DRY-RUN COMPLETE (NO CHANGES MADE) =====")
		return "dry-run-snapshot-id", nil
	}

	manifest := sdkbackup.SnapshotManifest{
		ID:          snapshotID,
		CreatedAt:   time.Now().UTC(),
		SourcePath:  "/",
		Deduplicate: false,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := m.Backend.WriteFile(filepath.Join(tmpPath, "manifest.json"), manifestJSON, 0644); err != nil {
		return "", fmt.Errorf("failed to write manifest: %w", err)
	}

	finalPath := filepath.Join(m.Backend.BasePath(), "snapshots", snapshotID)
	if err := m.Backend.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("failed to atomically rename snapshot: %w", err)
	}

	if m.RepoMgr.Config.RetentionKeepLast > 0 {
		m.App.Log.Term.Info().Msgf("Applying retention policy (keep last %d)", m.RepoMgr.Config.RetentionKeepLast)
		if err := m.RepoMgr.PruneOld(m.RepoMgr.Config.RetentionKeepLast); err != nil {
			m.App.Log.Term.Warn().Msgf("Retention pruning failed (non-critical): %v", err)
		}
	}

	m.App.Log.Term.Info().Msgf("===== BACKUP COMPLETE: %s =====", snapshotID)
	return snapshotID, nil
}

// generateSnapshotID generates a snapshot ID in the same format as the SDK.
func generateSnapshotID() string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	b := make([]byte, 4)
	rand.Read(b) //nolint:errcheck
	return fmt.Sprintf("%s-%x", ts, b)
}

// ListBackups lists all available backups
func (m *Manager) ListBackups(detailed bool) error {
	snapshots, err := m.RepoMgr.ListSnapshots()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No backups found.")
		return nil
	}

	headers := []string{"ID", "Created", "Size", "Dedup", "Providers"}
	var rows [][]string

	for _, snapshot := range snapshots {
		size := "-"
		if s, err := m.RepoMgr.GetSnapshotSize(snapshot.ID); err == nil {
			size = formatBytes(s)
		}

		providerList := "-"
		if p, err := m.RepoMgr.GetSnapshotProviders(snapshot.ID); err == nil && len(p) > 0 {
			providerList = strings.Join(p, ", ")
		}

		dedup := "no"
		if snapshot.Deduplicate {
			dedup = "yes"
		}

		rows = append(rows, []string{
			snapshot.ID,
			snapshot.CreatedAt.Format(time.RFC3339),
			size,
			dedup,
			providerList,
		})
	}

	return m.App.CLI.Table(headers, rows)
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

	size := "-"
	if s, err := m.RepoMgr.GetSnapshotSize(target.ID); err == nil {
		size = formatBytes(s)
	}

	dedup := "no"
	if target.Deduplicate {
		dedup = "yes"
	}

	fmt.Printf("\nSnapshot: %s\n\n", target.ID)
	m.App.CLI.Table(
		[]string{"Field", "Value"},
		[][]string{
			{"Created", target.CreatedAt.Format(time.RFC3339)},
			{"Source", target.SourcePath},
			{"Deduplication", dedup},
			{"Size", size},
		},
	)

	providerNames, err := m.RepoMgr.GetSnapshotProviders(target.ID)
	if err != nil {
		m.App.Log.Term.Warn().Msgf("Could not determine providers: %v", err)
		return nil
	}

	for _, provider := range providerNames {
		content, err := m.RepoMgr.GetProviderContent(target.ID, provider)
		if err != nil {
			m.App.Log.Term.Warn().Msgf("%s: could not read content: %v", provider, err)
			continue
		}

		fmt.Printf("\n%s\n\n", provider)
		if len(content.Rows) == 0 {
			fmt.Println("  (no content)")
			continue
		}
		m.App.CLI.Table(content.Headers, content.Rows)
	}

	fmt.Println()
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
