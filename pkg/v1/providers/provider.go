package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Provider interface for backup sources.
*/

import (
	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
)

// BackupProvider defines the interface for backup sources
type BackupProvider interface {
	// Name returns the provider name
	Name() string

	// Backup writes provider data directly to destPath on the backend.
	// For remote backends destPath is a remote path; large data providers must
	// use backend.CopyFromNative to avoid local staging.
	Backup(app *app.App, backend storage.Backend, destPath string) error

	// Restore reads provider data from sourcePath on the backend.
	Restore(app *app.App, backend storage.Backend, sourcePath string) error
}
