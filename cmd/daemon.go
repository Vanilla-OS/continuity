package cmd

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Daemon command for DBus service.
*/

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vanilla-os/continuity/pkg/v1/dbus"
	"github.com/vanilla-os/sdk/pkg/v1/cli"
)

// DaemonCmd starts the DBus daemon
type DaemonCmd struct {
	cli.Base
}

// Run executes the daemon command
func (c *DaemonCmd) Run() error {
	globalApp.Log.Term.Info().Msg("Starting Vanilla Continuity DBus daemon...")

	service, err := dbus.NewService(globalApp, globalCore)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	if err := service.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	globalApp.Log.Term.Info().Msg("DBus service started. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	globalApp.Log.Term.Info().Msg("Shutting down...")
	service.Stop()
	return nil
}
