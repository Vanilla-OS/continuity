package cmd

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Root command for Vanilla Continuity CLI.
*/

import (
	"fmt"

	"github.com/vanilla-os/continuity/pkg/v1/backup"
	"github.com/vanilla-os/continuity/pkg/v1/continuity"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/continuity/pkg/v1/restore"
	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/cli"
)

var globalApp *app.App
var globalCore *continuity.Core

// RootCmd represents the root command for Vanilla Continuity
type RootCmd struct {
	cli.Base
	Version VersionCmd `cmd:"version" help:"Show version information"`
	Status  StatusCmd  `cmd:"status" help:"Show Continuity status"`
	Backup  BackupCmd  `cmd:"backup" help:"Create a new backup"`
	List    ListCmd    `cmd:"list" help:"List all backups"`
	Inspect InspectCmd `cmd:"inspect" help:"Inspect a backup snapshot"`
	Restore RestoreCmd `cmd:"restore" help:"Restore from a backup"`
	Prune   PruneCmd   `cmd:"prune" help:"Prune old backups"`
	Daemon  DaemonCmd  `cmd:"daemon" help:"Start DBus daemon"`
}

// NewRootCmd creates a new root command
func NewRootCmd(application *app.App, core *continuity.Core) *RootCmd {
	globalApp = application
	globalCore = core
	return &RootCmd{}
}

// newBackend creates, connects and returns the configured storage backend.
// The caller is responsible for calling backend.Close().
func newBackend() (storage.Backend, error) {
	backend, err := storage.NewBackend(globalCore.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}
	if err := backend.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect storage backend: %w", err)
	}
	return backend, nil
}

// VersionCmd shows version information
type VersionCmd struct {
	cli.Base
}

// Run executes the version command
func (c *VersionCmd) Run() error {
	globalApp.Log.Term.Info().Msg("Vanilla Continuity v1.0.0")
	return nil
}

// StatusCmd shows Continuity status
type StatusCmd struct {
	cli.Base
}

// Run executes the status command
func (c *StatusCmd) Run() error {
	dedup := "no"
	if globalCore.Config.DefaultDeduplicate {
		dedup = "yes"
	}

	fmt.Println("\nContinuity Status")
	return globalApp.CLI.Table(
		[]string{"Setting", "Value"},
		[][]string{
			{"Status", "Ready"},
			{"Repository", globalCore.Config.RepositoryPath},
			{"Deduplication", dedup},
			{"Retention (keep last)", fmt.Sprintf("%d", globalCore.Config.RetentionKeepLast)},
		},
	)
}

// BackupCmd creates a new backup
type BackupCmd struct {
	cli.Base
	Label  string `arg:"" help:"Backup label" default:"manual"`
	DryRun bool   `name:"dry-run" help:"Simulate backup without making changes"`
}

// Run executes the backup command
func (c *BackupCmd) Run() error {
	backend, err := newBackend()
	if err != nil {
		return err
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(globalApp, globalCore.Config, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	backupMgr := backup.NewManager(globalApp, repoMgr, globalCore.Config.ExcludePatterns, globalCore.Config.EnabledProviders, backend, c.DryRun)
	snapshotID, err := backupMgr.RunBackup(c.Label)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	if c.DryRun {
		globalApp.Log.Term.Info().Msg("[DRY-RUN] Completed successfully (no changes made)")
	} else {
		globalApp.Log.Term.Info().Msgf("Backup created successfully: %s", snapshotID)
	}
	return nil
}

// ListCmd lists all backups
type ListCmd struct {
	cli.Base
	Details bool `name:"details" help:"Show detailed information for each backup"`
}

// Run executes the list command
func (c *ListCmd) Run() error {
	backend, err := newBackend()
	if err != nil {
		return err
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(globalApp, globalCore.Config, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	backupMgr := backup.NewManager(globalApp, repoMgr, globalCore.Config.ExcludePatterns, globalCore.Config.EnabledProviders, backend, false)
	if err := backupMgr.ListBackups(c.Details); err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	return nil
}

// RestoreCmd restores from a backup
type RestoreCmd struct {
	cli.Base
	SnapshotID string `arg:"" help:"Snapshot ID to restore"`
	DryRun     bool   `name:"dry-run" help:"Simulate restore without making changes"`
}

// Run executes the restore command
func (c *RestoreCmd) Run() error {
	backend, err := newBackend()
	if err != nil {
		return err
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(globalApp, globalCore.Config, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	restoreMgr := restore.NewManager(globalApp, repoMgr, backend, globalCore.Config.EnabledProviders, c.DryRun)
	if err := restoreMgr.RunRestore(c.SnapshotID); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	if c.DryRun {
		globalApp.Log.Term.Info().Msg("[DRY-RUN] Completed successfully (no changes made)")
	} else {
		globalApp.Log.Term.Info().Msg("Restore completed successfully")
	}
	return nil
}

// PruneCmd prunes old backups
type PruneCmd struct {
	cli.Base
	KeepLast int  `name:"keep-last" help:"Number of snapshots to keep" default:"7"`
	DryRun   bool `name:"dry-run" help:"Show what would be deleted without deleting"`
}

// Run executes the prune command
func (c *PruneCmd) Run() error {
	backend, err := newBackend()
	if err != nil {
		return err
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(globalApp, globalCore.Config, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	if c.DryRun {
		snapshots, err := repoMgr.ListSnapshots()
		if err != nil {
			return fmt.Errorf("failed to list snapshots: %w", err)
		}

		if len(snapshots) <= c.KeepLast {
			globalApp.Log.Term.Info().Msgf("No snapshots to prune (current: %d, keep: %d)", len(snapshots), c.KeepLast)
			return nil
		}

		toDelete := len(snapshots) - c.KeepLast
		globalApp.Log.Term.Info().Msgf("[DRY-RUN] Would delete %d snapshots (keeping last %d)", toDelete, c.KeepLast)
		for i := 0; i < toDelete; i++ {
			globalApp.Log.Term.Info().Msgf("  - %s (%s)", snapshots[i].ID, snapshots[i].CreatedAt)
		}
		return nil
	}

	if err := repoMgr.PruneOld(c.KeepLast); err != nil {
		return fmt.Errorf("prune failed: %w", err)
	}

	globalApp.Log.Term.Info().Msgf("Prune completed (kept last %d snapshots)", c.KeepLast)
	return nil
}

// InspectCmd inspects a backup snapshot
type InspectCmd struct {
	cli.Base
	SnapshotID string `arg:"" help:"Snapshot ID to inspect"`
}

// Run executes the inspect command
func (c *InspectCmd) Run() error {
	backend, err := newBackend()
	if err != nil {
		return err
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(globalApp, globalCore.Config, backend)
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	backupMgr := backup.NewManager(globalApp, repoMgr, globalCore.Config.ExcludePatterns, globalCore.Config.EnabledProviders, backend, false)
	if err := backupMgr.InspectBackup(c.SnapshotID); err != nil {
		return fmt.Errorf("failed to inspect backup: %w", err)
	}

	return nil
}
