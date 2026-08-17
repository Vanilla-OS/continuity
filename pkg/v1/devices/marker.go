package devices

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Continuity repository marker file read/write helpers.
*/

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentMarkerVersion is stamped on newly created markers.
const CurrentMarkerVersion = 1

// WriteMarker writes a Continuity repository marker at mountpoint.
func WriteMarker(mountpoint string, m RepoMarker) error {
	if m.Version == 0 {
		m.Version = CurrentMarkerVersion
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	target := filepath.Join(mountpoint, MarkerFile)
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	return nil
}

// LoadMarker reads the marker at mountpoint, returning false when missing.
func LoadMarker(mountpoint string) (RepoMarker, bool, error) {
	b, err := os.ReadFile(filepath.Join(mountpoint, MarkerFile))
	if err != nil {
		if os.IsNotExist(err) {
			return RepoMarker{}, false, nil
		}
		return RepoMarker{}, false, err
	}
	var m RepoMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return RepoMarker{}, false, fmt.Errorf("parse marker: %w", err)
	}
	return m, true, nil
}
