package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: ABRoot provider backs up ABRoot metadata.
*/

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
)

// ABRootProvider backs up ABRoot metadata
type ABRootProvider struct{}

// NewABRootProvider creates a new ABRoot provider
func NewABRootProvider() *ABRootProvider {
	return &ABRootProvider{}
}

// Name returns the provider name
func (p *ABRootProvider) Name() string {
	return "ABRoot"
}

// Backup backs up ABRoot metadata from /etc/abroot
func (p *ABRootProvider) Backup(app *app.App) (string, error) {
	app.Log.Term.Info().Msg("Starting ABRoot metadata backup...")

	tmpDir, err := os.MkdirTemp("", "continuity-abroot-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	abrootSource := "/etc/abroot"
	abrootDest := filepath.Join(tmpDir, "abroot")

	if _, err := os.Stat(abrootSource); os.IsNotExist(err) {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("ABRoot metadata not found at %s", abrootSource)
	}

	app.Log.Term.Info().Msgf("Copying %s to %s", abrootSource, abrootDest)

	copyOpts := fs.CopyTreeOptions{
		Workers:             1,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	if err := fs.CopyTree(abrootSource, abrootDest, copyOpts); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to copy ABRoot metadata: %w", err)
	}

	app.Log.Term.Info().Msgf("ABRoot metadata backup staged at %s", tmpDir)
	return tmpDir, nil
}

// Restore restores ABRoot metadata to /etc/abroot
func (p *ABRootProvider) Restore(app *app.App, sourcePath string) error {
	app.Log.Term.Info().Msg("Starting ABRoot metadata restore...")

	abrootSrc := filepath.Join(sourcePath, "abroot")
	abrootDst := "/etc/abroot"

	if _, err := os.Stat(abrootSrc); os.IsNotExist(err) {
		return fmt.Errorf("backup does not contain ABRoot metadata")
	}

	app.Log.Term.Info().Msgf("Restoring %s to %s", abrootSrc, abrootDst)

	copyOpts := fs.CopyTreeOptions{
		Workers:             1,
		PreserveOwnership:   true,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	if err := fs.CopyTree(abrootSrc, abrootDst, copyOpts); err != nil {
		return fmt.Errorf("failed to restore ABRoot metadata: %w", err)
	}

	app.Log.Term.Info().Msg("ABRoot metadata restore completed")
	return nil
}
