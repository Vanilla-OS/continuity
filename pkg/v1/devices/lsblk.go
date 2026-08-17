package devices

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: lsblk(8) JSON parser used to enumerate block devices.
*/

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type lsblkOutput struct {
	Blockdevices []lsblkNode `json:"blockdevices"`
}

type lsblkNode struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Kname       string      `json:"kname"`
	Pkname      string      `json:"pkname"`
	Type        string      `json:"type"`
	FSType      string      `json:"fstype"`
	Label       string      `json:"label"`
	UUID        string      `json:"uuid"`
	Size        flexUint64  `json:"size"`
	Model       string      `json:"model"`
	Vendor      string      `json:"vendor"`
	Serial      string      `json:"serial"`
	Tran        string      `json:"tran"`
	RM          flexBool    `json:"rm"`
	Hotplug     flexBool    `json:"hotplug"`
	RO          flexBool    `json:"ro"`
	Mountpoint  string      `json:"mountpoint"`
	Mountpoints []string    `json:"mountpoints"`
	Children    []lsblkNode `json:"children,omitempty"`
}

// flexBool accepts both JSON booleans and the legacy "0"/"1" strings emitted
// by older util-linux releases.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	switch s {
	case "true", "1":
		*f = true
	case "false", "0", "", "null":
		*f = false
	default:
		return fmt.Errorf("invalid bool: %s", s)
	}
	return nil
}

// flexUint64 accepts both JSON numbers and strings (older lsblk versions emit
// SIZE as a quoted byte count even with --bytes).
type flexUint64 uint64

func (n *flexUint64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*n = flexUint64(v)
	return nil
}

func runLsblk() (*lsblkOutput, error) {
	cmd := exec.Command("lsblk",
		"-J", "-b",
		"-o", "NAME,PATH,KNAME,PKNAME,TYPE,FSTYPE,LABEL,UUID,SIZE,MODEL,VENDOR,SERIAL,TRAN,RM,HOTPLUG,RO,MOUNTPOINT,MOUNTPOINTS",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}
	var data lsblkOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("parse lsblk output: %w", err)
	}
	return &data, nil
}

func flatten(o *lsblkOutput) []Device {
	var out []Device
	var walk func(n lsblkNode, inheritedTran string)
	walk = func(n lsblkNode, inheritedTran string) {
		tran := n.Tran
		if tran == "" {
			tran = inheritedTran
		}
		mp := n.Mountpoint
		if mp == "" {
			for _, m := range n.Mountpoints {
				if m != "" {
					mp = m
					break
				}
			}
		}
		out = append(out, Device{
			Path:        n.Path,
			Kname:       n.Kname,
			Pkname:      n.Pkname,
			Type:        n.Type,
			FSType:      n.FSType,
			Label:       n.Label,
			UUID:        n.UUID,
			Size:        uint64(n.Size),
			Model:       n.Model,
			Vendor:      n.Vendor,
			Serial:      n.Serial,
			Transport:   tran,
			Removable:   bool(n.RM),
			Hotplug:     bool(n.Hotplug),
			ReadOnly:    bool(n.RO),
			Mountpoint:  mp,
			Mountpoints: n.Mountpoints,
		})
		for _, c := range n.Children {
			walk(c, tran)
		}
	}
	for _, n := range o.Blockdevices {
		walk(n, "")
	}
	return out
}
