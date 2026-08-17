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
	"github.com/vanilla-os/continuity/pkg/v1/devices"
	"github.com/vanilla-os/continuity/pkg/v1/repo"
	"github.com/vanilla-os/continuity/pkg/v1/repoinit"
	"github.com/vanilla-os/continuity/pkg/v1/restore"
	"github.com/vanilla-os/continuity/pkg/v1/runtime"
	"github.com/vanilla-os/continuity/pkg/v1/storage"
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
<method name="ListDevices">
<arg direction="in" type="b" name="include_all"/>
<arg direction="out" type="aa{sv}" name="devices"/>
</method>
<method name="GetDeviceInfo">
<arg direction="in" type="s" name="device_path"/>
<arg direction="out" type="a{sv}" name="info"/>
</method>
<method name="InitRepository">
<arg direction="in" type="s" name="device_path"/>
<arg direction="in" type="s" name="label"/>
<arg direction="in" type="b" name="encrypt"/>
<arg direction="in" type="s" name="fs"/>
<arg direction="in" type="s" name="password"/>
<arg direction="in" type="b" name="activate"/>
<arg direction="out" type="s" name="mount_path"/>
</method>
<method name="UnlockRepository">
<arg direction="in" type="s" name="device_path"/>
<arg direction="in" type="s" name="password"/>
<arg direction="out" type="s" name="mount_path"/>
</method>
<method name="LockRepository">
<arg direction="in" type="s" name="device_path"/>
<arg direction="out" type="b" name="success"/>
</method>
<method name="GetActiveRepository">
<arg direction="out" type="a{sv}" name="info"/>
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

// newBackend creates and connects a storage backend for the current config.
func (s *Service) newBackend() (storage.Backend, error) {
	backend, err := storage.NewBackend(s.Core.EffectiveConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}
	if err := backend.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect storage backend: %w", err)
	}
	return backend, nil
}

// CreateBackup creates a new backup (DBus method)
func (s *Service) CreateBackup(label string) (string, *dbus.Error) {
	s.App.Log.Term.Info().Msgf("DBus: CreateBackup called with label=%s", label)

	backend, err := s.newBackend()
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("failed to init backend: %w", err))
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(s.App, s.Core.Config, backend)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("failed to init repo: %w", err))
	}

	backupMgr := backup.NewManager(s.App, repoMgr, s.Core.Config.ExcludePatterns, s.Core.Config.EnabledProviders, backend, false)
	snapshotID, err := backupMgr.RunBackup(label)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("backup failed: %w", err))
	}

	return snapshotID, nil
}

// ListBackups lists all backups (DBus method)
func (s *Service) ListBackups() ([]string, *dbus.Error) {
	s.App.Log.Term.Info().Msg("DBus: ListBackups called")

	backend, err := s.newBackend()
	if err != nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("failed to init backend: %w", err))
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(s.App, s.Core.Config, backend)
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

	backend, err := s.newBackend()
	if err != nil {
		return false, dbus.MakeFailedError(fmt.Errorf("failed to init backend: %w", err))
	}
	defer backend.Close()

	repoMgr, err := repo.NewManager(s.App, s.Core.Config, backend)
	if err != nil {
		return false, dbus.MakeFailedError(fmt.Errorf("failed to init repo: %w", err))
	}

	restoreMgr := restore.NewManager(s.App, repoMgr, backend, s.Core.Config.EnabledProviders, false)
	if err := restoreMgr.RunRestore(snapshotID); err != nil {
		return false, dbus.MakeFailedError(fmt.Errorf("restore failed: %w", err))
	}

	return true, nil
}

// GetStatus returns the service status (DBus method)
func (s *Service) GetStatus() (string, *dbus.Error) {
	return "ready", nil
}

// ListDevices returns the candidate (or all) block devices visible to the
// system (DBus method).
func (s *Service) ListDevices(includeAll bool) ([]map[string]dbus.Variant, *dbus.Error) {
	devs, err := devices.List(devices.ListOptions{All: includeAll})
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	out := make([]map[string]dbus.Variant, 0, len(devs))
	for _, d := range devs {
		out = append(out, deviceToVariant(d))
	}
	return out, nil
}

// GetDeviceInfo returns detailed information for a single device (DBus method).
func (s *Service) GetDeviceInfo(devicePath string) (map[string]dbus.Variant, *dbus.Error) {
	devs, err := devices.List(devices.ListOptions{All: true})
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	for _, d := range devs {
		if d.Path == devicePath {
			return deviceToVariant(d), nil
		}
	}
	return nil, dbus.MakeFailedError(fmt.Errorf("device not found: %s", devicePath))
}

// InitRepository formats a device as a Continuity repository (DBus method).
// Pass password="" together with encrypt=false to create a plaintext repo.
func (s *Service) InitRepository(devicePath, label string, encrypt bool, fs, password string, activate bool) (string, *dbus.Error) {
	s.App.Log.Term.Info().Msgf("DBus: InitRepository device=%s encrypt=%v fs=%s", devicePath, encrypt, fs)

	repository, err := repoinit.InitRepository(repoinit.InitOptions{
		DevicePath: devicePath,
		Label:      label,
		Encrypt:    encrypt,
		FS:         fs,
		Password:   password,
	})
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}

	if !activate {
		if err := repository.Close(); err != nil {
			return "", dbus.MakeFailedError(fmt.Errorf("failed to lock new repository: %w", err))
		}
		return repository.MountPath, nil
	}

	if err := runtime.SetActive(runtime.ActiveState{
		DevicePath: repository.DevicePath,
		MountPath:  repository.MountPath,
		RepoPath:   repository.MountPath,
		LUKS:       repository.LUKS,
		Label:      repository.Marker.Label,
		UUID:       repository.Marker.UUID,
	}); err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("failed to persist active state: %w", err))
	}
	return repository.MountPath, nil
}

// UnlockRepository opens an existing repository and marks it as active.
func (s *Service) UnlockRepository(devicePath, password string) (string, *dbus.Error) {
	s.App.Log.Term.Info().Msgf("DBus: UnlockRepository device=%s", devicePath)

	repository, err := repoinit.UnlockRepository(repoinit.UnlockOptions{
		DevicePath: devicePath,
		Password:   password,
	})
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}

	if err := runtime.SetActive(runtime.ActiveState{
		DevicePath: repository.DevicePath,
		MountPath:  repository.MountPath,
		RepoPath:   repository.MountPath,
		LUKS:       repository.LUKS,
		Label:      repository.Marker.Label,
		UUID:       repository.Marker.UUID,
	}); err != nil {
		_ = repository.Close()
		return "", dbus.MakeFailedError(fmt.Errorf("failed to persist active state: %w", err))
	}
	return repository.MountPath, nil
}

// LockRepository locks and unmounts the named (or active, if empty) device.
func (s *Service) LockRepository(devicePath string) (bool, *dbus.Error) {
	target := devicePath
	if target == "" {
		active, err := runtime.GetActive()
		if err != nil {
			return false, dbus.MakeFailedError(err)
		}
		if active == nil {
			return false, dbus.MakeFailedError(fmt.Errorf("no active repository to lock"))
		}
		target = active.DevicePath
	}

	if err := repoinit.LockDevice(target); err != nil {
		return false, dbus.MakeFailedError(err)
	}
	if active, err := runtime.GetActive(); err == nil && active != nil && active.DevicePath == target {
		_ = runtime.ClearActive()
	}
	return true, nil
}

// GetActiveRepository returns the current active runtime repository, or an
// empty dict when none is set.
func (s *Service) GetActiveRepository() (map[string]dbus.Variant, *dbus.Error) {
	active, err := runtime.GetActive()
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	if active == nil {
		return map[string]dbus.Variant{}, nil
	}
	return map[string]dbus.Variant{
		"device_path": dbus.MakeVariant(active.DevicePath),
		"mount_path":  dbus.MakeVariant(active.MountPath),
		"repo_path":   dbus.MakeVariant(active.RepoPath),
		"luks":        dbus.MakeVariant(active.LUKS),
		"label":       dbus.MakeVariant(active.Label),
		"uuid":        dbus.MakeVariant(active.UUID),
		"opened_at":   dbus.MakeVariant(active.OpenedAt.Format("2006-01-02T15:04:05Z07:00")),
	}, nil
}

func deviceToVariant(d devices.Device) map[string]dbus.Variant {
	m := map[string]dbus.Variant{
		"path":          dbus.MakeVariant(d.Path),
		"type":          dbus.MakeVariant(d.Type),
		"fstype":        dbus.MakeVariant(d.FSType),
		"label":         dbus.MakeVariant(d.Label),
		"uuid":          dbus.MakeVariant(d.UUID),
		"size":          dbus.MakeVariant(d.Size),
		"model":         dbus.MakeVariant(d.Model),
		"vendor":        dbus.MakeVariant(d.Vendor),
		"serial":        dbus.MakeVariant(d.Serial),
		"transport":     dbus.MakeVariant(d.Transport),
		"removable":     dbus.MakeVariant(d.Removable),
		"hotplug":       dbus.MakeVariant(d.Hotplug),
		"readonly":      dbus.MakeVariant(d.ReadOnly),
		"mountpoint":    dbus.MakeVariant(d.Mountpoint),
		"is_luks":       dbus.MakeVariant(d.IsLUKS),
		"is_continuity": dbus.MakeVariant(d.IsContinuity),
	}
	if d.ContinuityInfo != nil {
		m["repo_label"] = dbus.MakeVariant(d.ContinuityInfo.Label)
		m["repo_uuid"] = dbus.MakeVariant(d.ContinuityInfo.UUID)
		m["repo_created_at"] = dbus.MakeVariant(d.ContinuityInfo.CreatedAt)
	}
	return m
}

// Stop stops the DBus service
func (s *Service) Stop() {
	if s.conn != nil {
		s.conn.Close()
	}
}
