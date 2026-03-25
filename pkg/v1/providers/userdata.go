package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: UserData provider backs up user home directories.
*/

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
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

// Backup backs up all user home directories to a temporary location
func (p *UserDataProvider) Backup(app *app.App) (string, error) {
	app.Log.Term.Info().Msg("Starting UserData backup...")

	tmpDir, err := os.MkdirTemp("", "continuity-userdata-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	homeSource := "/home"
	homeDest := filepath.Join(tmpDir, "home")

	app.Log.Term.Info().Msgf("Copying %s to %s", homeSource, homeDest)

	copyOpts := fs.CopyTreeOptions{
		Workers:             2,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	if err := fs.CopyTree(homeSource, homeDest, copyOpts); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to copy %s: %w", homeSource, err)
	}

	app.Log.Term.Info().Msgf("UserData backup staged at %s", tmpDir)
	return tmpDir, nil
}

// Restore restores user home directories from backup
func (p *UserDataProvider) Restore(app *app.App, sourcePath string) error {
	app.Log.Term.Info().Msg("Starting UserData restore...")

	homeSrc := filepath.Join(sourcePath, "home")
	homeDst := "/home"

	if _, err := os.Stat(homeSrc); os.IsNotExist(err) {
		return fmt.Errorf("backup does not contain home directory")
	}

	app.Log.Term.Info().Msgf("Restoring %s to %s", homeSrc, homeDst)

	copyOpts := fs.CopyTreeOptions{
		Workers:             2,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	if err := fs.CopyTree(homeSrc, homeDst, copyOpts); err != nil {
		return fmt.Errorf("failed to restore home directory: %w", err)
	}

	app.Log.Term.Info().Msg("UserData restore completed")
	return nil
}
