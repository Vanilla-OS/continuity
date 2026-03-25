package providers

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Flatpak provider backs up installed Flatpak applications.
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vanilla-os/sdk/pkg/v1/app"
)

// FlatpakApp represents a Flatpak application
type FlatpakApp struct {
	Name   string `json:"name"`
	Ref    string `json:"ref"`
	Origin string `json:"origin"`
	Branch string `json:"branch"`
}

// FlatpakProvider backs up Flatpak application list
type FlatpakProvider struct{}

// NewFlatpakProvider creates a new Flatpak provider
func NewFlatpakProvider() *FlatpakProvider {
	return &FlatpakProvider{}
}

// Name returns the provider name
func (p *FlatpakProvider) Name() string {
	return "Flatpak"
}

// Backup lists installed Flatpak apps and saves to JSON
func (p *FlatpakProvider) Backup(app *app.App) (string, error) {
	app.Log.Term.Info().Msg("Starting Flatpak backup...")

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "continuity-flatpak-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// List installed Flatpaks
	cmd := exec.Command("flatpak", "list", "--app", "--columns=name,application,origin,branch")
	output, err := cmd.Output()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to list flatpaks: %w", err)
	}

	// Parse output
	var apps []FlatpakApp
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		apps = append(apps, FlatpakApp{
			Name:   strings.TrimSpace(fields[0]),
			Ref:    strings.TrimSpace(fields[1]),
			Origin: strings.TrimSpace(fields[2]),
			Branch: strings.TrimSpace(fields[3]),
		})
	}

	// Save to JSON
	listPath := filepath.Join(tmpDir, "flatpak-apps.json")
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to marshal flatpak list: %w", err)
	}

	if err := os.WriteFile(listPath, data, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to write flatpak list: %w", err)
	}

	app.Log.Term.Info().Msgf("Flatpak backup staged: %d apps", len(apps))
	return tmpDir, nil
}

// Restore reinstalls Flatpak apps from backup
func (p *FlatpakProvider) Restore(app *app.App, sourcePath string) error {
	app.Log.Term.Info().Msg("Starting Flatpak restore...")

	listPath := filepath.Join(sourcePath, "flatpak-apps.json")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return fmt.Errorf("failed to read flatpak list: %w", err)
	}

	var apps []FlatpakApp
	if err := json.Unmarshal(data, &apps); err != nil {
		return fmt.Errorf("failed to parse flatpak list: %w", err)
	}

	app.Log.Term.Info().Msgf("Restoring %d Flatpak applications...", len(apps))

	for _, flatpak := range apps {
		app.Log.Term.Info().Msgf("Installing %s (%s)", flatpak.Name, flatpak.Ref)
		cmd := exec.Command("flatpak", "install", "-y", "--noninteractive", flatpak.Origin, flatpak.Ref)
		if output, err := cmd.CombinedOutput(); err != nil {
			app.Log.Term.Warn().Msgf("Failed to install %s: %v\n%s", flatpak.Name, err, string(output))
		}
	}

	app.Log.Term.Info().Msg("Flatpak restore completed")
	return nil
}
