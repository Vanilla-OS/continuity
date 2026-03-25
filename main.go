package main

/*	License: GPLv3
	Authors:
		Vanilla OS Contributors <https://github.com/vanilla-os/>
	Copyright: 2026
	Description:
		Vanilla Continuity is a Time Machine-like backup system for
		Vanilla OS with ABRoot integration. Provides snapshot-based
		backups with deduplication, retention policies, and restore
		capabilities for user data, Flatpak apps, and ABRoot metadata.
*/

import (
	"fmt"
	"os"

	"github.com/vanilla-os/continuity/cmd"
	"github.com/vanilla-os/continuity/pkg/v1/continuity"
	"github.com/vanilla-os/sdk/pkg/v1/app"
	"github.com/vanilla-os/sdk/pkg/v1/app/types"
)

var continuityApp *app.App

func main() {
	var err error
	continuityApp, err = app.NewApp(types.AppOptions{
		RDNN:    "org.vanillaos.Continuity",
		Name:    "Vanilla Continuity",
		Version: "1.0.0",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	// Initialize Continuity core
	core, err := continuity.NewCore(continuityApp)
	if err != nil {
		continuityApp.Log.Term.Error().Msgf("Failed to initialize Continuity core: %v", err)
		os.Exit(1)
	}

	// Setup CLI
	rootCmd := cmd.NewRootCmd(continuityApp, core)
	err = continuityApp.WithCLI(rootCmd)
	if err != nil {
		continuityApp.Log.Term.Error().Msgf("Failed to setup CLI: %v", err)
		os.Exit(1)
	}

	// Execute CLI
	continuityApp.CLI.Execute()
}
