package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: UserData provider backs up user home directories.
*/

import (
	"fmt"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
)

// UserDataProvider backs up user home directories
type UserDataProvider struct {
	ExcludePatterns []string
}

// NewUserDataProvider creates a new UserData provider
func NewUserDataProvider(excludePatterns []string) *UserDataProvider {
	return &UserDataProvider{
		ExcludePatterns: excludePatterns,
	}
}

// Name returns the provider name
func (p *UserDataProvider) Name() string {
	return "UserData"
}

// Backup copies /home directly to destPath on the backend without local staging.
func (p *UserDataProvider) Backup(app *app.App, backend storage.Backend, destPath string) error {
	app.Log.Term.Info().Msg("Starting UserData backup...")

	homeSource := "/home"
	homeDest := filepath.Join(destPath, "home")

	app.Log.Term.Info().Msgf("Copying %s to %s", homeSource, homeDest)

	if err := backend.CopyFromNative(homeSource, homeDest); err != nil {
		return fmt.Errorf("failed to copy %s: %w", homeSource, err)
	}

	app.Log.Term.Info().Msg("UserData backup completed")
	return nil
}

// Restore restores user home directories from backup.
func (p *UserDataProvider) Restore(app *app.App, backend storage.Backend, sourcePath string) error {
	app.Log.Term.Info().Msg("Starting UserData restore...")

	homeSrc := filepath.Join(sourcePath, "home")
	homeDst := "/home"

	app.Log.Term.Info().Msgf("Restoring %s to %s", homeSrc, homeDst)

	if err := backend.CopyToNative(homeSrc, homeDst); err != nil {
		return fmt.Errorf("failed to restore home directory: %w", err)
	}

	app.Log.Term.Info().Msg("UserData restore completed")
	return nil
}
