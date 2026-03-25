package dbus

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: DBus service for Vanilla Continuity.
*/

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/vanilla-os/continuity/pkg/v1/backup"
	"github.com/vanilla-os/continuity/pkg/v1/continuity"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/continuity/pkg/v1/restore"
	"github.com/vanilla-os/sdk/pkg/v1/app"
)

const (
	dbusPath      = "/org/vanillaos/Continuity"
	dbusInterface = "org.vanillaos.Continuity"
	introspectXML = `
<node>
<interface name="org.vanillaos.Continuity">
<method name="CreateBackup">
<arg direction="in" type="s" name="label"/>
<arg direction="out" type="s" name="snapshot_id"/>
</method>
<method name="ListBackups">
<arg direction="out" type="as" name="snapshot_ids"/>
</method>
<method name="RestoreBackup">
<arg direction="in" type="s" name="snapshot_id"/>
<arg direction="out" type="b" name="success"/>
</method>
<method name="GetStatus">
<arg direction="out" type="s" name="status"/>
</method>
</interface>
` + introspect.IntrospectDataString + `
</node>`
)

// Service implements the DBus service for Continuity
type Service struct {
	App  *app.App
	Core *continuity.Core
	conn *dbus.Conn
}

// NewService creates a new DBus service
func NewService(app *app.App, core *continuity.Core) (*Service, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %w", err)
	}

	return &Service{
		App:  app,
		Core: core,
		conn: conn,
	}, nil
}

// Start starts the DBus service
func (s *Service) Start() error {
	reply, err := s.conn.RequestName(dbusInterface, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name already taken")
	}

	if err := s.conn.Export(s, dbus.ObjectPath(dbusPath), dbusInterface); err != nil {
		return fmt.Errorf("failed to export service: %w", err)
	}
	if err := s.conn.Export(introspect.Introspectable(introspectXML), dbus.ObjectPath(dbusPath), "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("failed to export introspection: %w", err)
	}

	s.App.Log.Term.Info().Msgf("DBus service started on %s", dbusInterface)
	return nil
}

// CreateBackup creates a new backup (DBus method)
func (s *Service) CreateBackup(label string) (string, *dbus.Error) {
	s.App.Log.Term.Info().Msgf("DBus: CreateBackup called with label=%s", label)

	repoMgr, err := repo.NewManager(s.App, s.Core.Config)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("failed to init repo: %w", err))
	}

	backupMgr := backup.NewManager(s.App, repoMgr, s.Core.Config.ExcludePatterns, false)
	snapshotID, err := backupMgr.RunBackup(label)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("backup failed: %w", err))
	}

	return snapshotID, nil
}

// ListBackups lists all backups (DBus method)
func (s *Service) ListBackups() ([]string, *dbus.Error) {
	s.App.Log.Term.Info().Msg("DBus: ListBackups called")

	repoMgr, err := repo.NewManager(s.App, s.Core.Config)
	if err != nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("failed to init repo: %w", err))
	}

	snapshots, err := repoMgr.ListSnapshots()
	if err != nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("failed to list: %w", err))
	}

	ids := make([]string, len(snapshots))
	for i, snap := range snapshots {
		ids[i] = snap.ID
	}

	return ids, nil
}

// RestoreBackup restores a backup (DBus method)
func (s *Service) RestoreBackup(snapshotID string) (bool, *dbus.Error) {
	s.App.Log.Term.Info().Msgf("DBus: RestoreBackup called with snapshot=%s", snapshotID)

	repoMgr, err := repo.NewManager(s.App, s.Core.Config)
	if err != nil {
		return false, dbus.MakeFailedError(fmt.Errorf("failed to init repo: %w", err))
	}

	restoreMgr := restore.NewManager(s.App, repoMgr, false)
	if err := restoreMgr.RunRestore(snapshotID); err != nil {
		return false, dbus.MakeFailedError(fmt.Errorf("restore failed: %w", err))
	}

	return true, nil
}

// GetStatus returns the service status (DBus method)
func (s *Service) GetStatus() (string, *dbus.Error) {
	return "ready", nil
}

// Stop stops the DBus service
func (s *Service) Stop() {
	if s.conn != nil {
		s.conn.Close()
	}
}
