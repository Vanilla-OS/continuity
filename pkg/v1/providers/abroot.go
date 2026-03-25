package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: ABRoot provider backs up ABRoot metadata.
*/

import (
	"fmt"
	"path/filepath"

	"github.com/vanilla-os/continuity/pkg/v1/storage"
	"github.com/vanilla-os/sdk/pkg/v1/app"
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

// Backup copies /etc/abroot directly to destPath on the backend without local staging.
func (p *ABRootProvider) Backup(app *app.App, backend storage.Backend, destPath string) error {
	app.Log.Term.Info().Msg("Starting ABRoot metadata backup...")

	abrootSource := "/etc/abroot"
	abrootDest := filepath.Join(destPath, "abroot")

	app.Log.Term.Info().Msgf("Copying %s to %s", abrootSource, abrootDest)

	if err := backend.CopyFromNative(abrootSource, abrootDest); err != nil {
		return fmt.Errorf("failed to copy ABRoot metadata: %w", err)
	}

	app.Log.Term.Info().Msg("ABRoot metadata backup completed")
	return nil
}

// Restore restores ABRoot metadata to /etc/abroot.
func (p *ABRootProvider) Restore(app *app.App, backend storage.Backend, sourcePath string) error {
	app.Log.Term.Info().Msg("Starting ABRoot metadata restore...")

	abrootSrc := filepath.Join(sourcePath, "abroot")
	abrootDst := "/etc/abroot"

	app.Log.Term.Info().Msgf("Restoring %s to %s", abrootSrc, abrootDst)

	if err := backend.CopyToNative(abrootSrc, abrootDst); err != nil {
		return fmt.Errorf("failed to restore ABRoot metadata: %w", err)
	}

	app.Log.Term.Info().Msg("ABRoot metadata restore completed")
	return nil
}
