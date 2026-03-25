package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Provider interface for backup sources.
*/

import "github.com/vanilla-os/sdk/pkg/v1/app"

// BackupProvider defines the interface for backup sources
type BackupProvider interface {
	// Name returns the provider name
	Name() string

	// Backup performs the backup operation
	// Returns the path to backup data and any error
	Backup(app *app.App) (string, error)

	// Restore performs the restore operation
	Restore(app *app.App, sourcePath string) error
}
