package runtime

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Active runtime repository pointer shared between CLI and daemon.
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateDir holds the active-repository JSON pointer.
const StateDir = "/run/continuity"

const stateFile = "active.json"

// ActiveState describes the currently-unlocked repository.
type ActiveState struct {
	DevicePath string    `json:"device_path"`
	MountPath  string    `json:"mount_path"`
	RepoPath   string    `json:"repo_path"`
	LUKS       bool      `json:"luks"`
	Label      string    `json:"label,omitempty"`
	UUID       string    `json:"uuid,omitempty"`
	OpenedAt   time.Time `json:"opened_at"`
}

// SetActive writes the active state, replacing any previous value.
func SetActive(s ActiveState) error {
	if err := os.MkdirAll(StateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if s.OpenedAt.IsZero() {
		s.OpenedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := filepath.Join(StateDir, stateFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return os.Rename(tmp, filepath.Join(StateDir, stateFile))
}

// GetActive returns the current state, or (nil, nil) when none is set.
func GetActive() (*ActiveState, error) {
	b, err := os.ReadFile(filepath.Join(StateDir, stateFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s ActiveState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// ClearActive removes the active-repository pointer.
func ClearActive() error {
	err := os.Remove(filepath.Join(StateDir, stateFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state: %w", err)
	}
	return nil
}
