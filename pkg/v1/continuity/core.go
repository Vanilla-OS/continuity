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
	"github.com/vanilla-os/continuity/pkg/v1/runtime"
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

// EffectiveConfig returns a Config with RepositoryPath overridden by the
// currently-active runtime repository, when one is set. The returned value
// is a shallow copy: the caller must not mutate slice fields.
func (c *Core) EffectiveConfig() *config.Config {
	cfg := *c.Config
	if active, err := runtime.GetActive(); err == nil && active != nil && active.RepoPath != "" {
		cfg.RepositoryPath = active.RepoPath
		// An active runtime repo is always local on this host.
		cfg.Remote = nil
	}
	return &cfg
}
