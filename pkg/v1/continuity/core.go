package continuity

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Core Continuity application logic and state management.
*/

import (
	"fmt"

	"github.com/vanilla-os/continuity/pkg/v1/config"
	"github.com/vanilla-os/sdk/pkg/v1/app"
)

// Core represents the Continuity application core
type Core struct {
	App    *app.App
	Config *config.Config
}

// NewCore creates a new Continuity core instance
func NewCore(app *app.App) (*Core, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return &Core{
		App:    app,
		Config: cfg,
	}, nil
}
