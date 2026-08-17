package devices

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: Block device enumeration and Continuity repository detection.
*/

import (
	"github.com/vanilla-os/continuity/pkg/v1/crypto"
)

// MarkerFile identifies a Continuity repository filesystem.
const MarkerFile = ".continuity-repo.json"

// Device is an lsblk node enriched with Continuity-specific detection.
type Device struct {
	Path        string
	Kname       string
	Pkname      string
	Type        string
	FSType      string
	Label       string
	UUID        string
	Size        uint64
	Model       string
	Vendor      string
	Serial      string
	Transport   string
	Removable   bool
	Hotplug     bool
	ReadOnly    bool
	Mountpoint  string
	Mountpoints []string

	IsLUKS         bool
	IsContinuity   bool
	ContinuityInfo *RepoMarker
}

// RepoMarker is the JSON payload of MarkerFile.
type RepoMarker struct {
	Version    int    `json:"version"`
	Label      string `json:"label,omitempty"`
	UUID       string `json:"uuid,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	DeviceHint string `json:"device_hint,omitempty"`
}

// ListOptions tunes which devices List returns.
type ListOptions struct {
	// All disables the candidate filter and returns every lsblk node.
	All bool
}

// List returns the block devices that can host a Continuity repository.
func List(opts ListOptions) ([]Device, error) {
	raw, err := runLsblk()
	if err != nil {
		return nil, err
	}
	flat := flatten(raw)
	out := make([]Device, 0, len(flat))
	for _, d := range flat {
		if !opts.All && !candidateForRepo(d) {
			continue
		}
		annotate(&d)
		out = append(out, d)
	}
	return out, nil
}

// candidateForRepo returns true when d can host a Continuity repository.
// Read-only nodes (including squashfs snap loops) and bare disks with
// partition children are excluded.
func candidateForRepo(d Device) bool {
	if d.ReadOnly {
		return false
	}
	switch d.Type {
	case "part", "crypt":
		return true
	case "disk", "loop":
		return d.FSType != ""
	}
	return false
}

func annotate(d *Device) {
	d.IsLUKS = d.FSType == "crypto_LUKS"

	if d.IsLUKS {
		if ok, tok, err := crypto.HasContinuityToken(d.Path); err == nil && ok {
			d.IsContinuity = true
			d.ContinuityInfo = &RepoMarker{
				Version:   tok.Version,
				Label:     tok.Label,
				UUID:      tok.UUID,
				CreatedAt: tok.CreatedAt,
			}
		}
	}

	if d.Mountpoint != "" {
		if m, ok, _ := LoadMarker(d.Mountpoint); ok {
			d.IsContinuity = true
			d.ContinuityInfo = &m
		}
	}
}
