package cmd

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: `continuity device` command group: discover and manage block
devices used as Continuity repository targets.
*/

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/vanilla-os/continuity/pkg/v1/devices"
	"github.com/vanilla-os/continuity/pkg/v1/repoinit"
	"github.com/vanilla-os/continuity/pkg/v1/runtime"
	"github.com/vanilla-os/sdk/pkg/v1/cli"
	"golang.org/x/term"
)

// DeviceCmd is the `continuity device` command group.
type DeviceCmd struct {
	cli.Base
	List   DeviceListCmd   `cmd:"list" help:"List candidate block devices for Continuity repositories"`
	Init   DeviceInitCmd   `cmd:"init" help:"Initialize a device as a Continuity repository (destroys data)"`
	Unlock DeviceUnlockCmd `cmd:"unlock" help:"Unlock and mount a Continuity repository, set it as active"`
	Lock   DeviceLockCmd   `cmd:"lock" help:"Lock and unmount the active (or named) Continuity repository"`
	Info   DeviceInfoCmd   `cmd:"info" help:"Show details about a device"`
}

// DeviceListCmd implements `continuity device list`.
type DeviceListCmd struct {
	cli.Base
	All bool `cli:"all" help:"Include internal, read-only and bare disks"`
}

// Run executes the list command.
func (c *DeviceListCmd) Run() error {
	devs, err := devices.List(devices.ListOptions{All: c.All})
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	rows := make([][]string, 0, len(devs))
	for _, d := range devs {
		rows = append(rows, []string{
			d.Path,
			d.Type,
			humanSize(d.Size),
			fsLabel(d),
			transportLabel(d),
			mountLabel(d),
			repoLabel(d),
		})
	}

	fmt.Println("\nContinuity Devices")
	return globalApp.CLI.Table(
		[]string{"Device", "Type", "Size", "Filesystem", "Bus", "Mountpoint", "Continuity"},
		rows,
	)
}

// DeviceInitCmd implements `continuity device init`.
type DeviceInitCmd struct {
	cli.Base
	Device     string `arg:"" help:"Block device to initialize (e.g. /dev/sdb1)"`
	Label      string `cli:"label" help:"Human-readable repository label"`
	FS         string `cli:"fs" help:"Filesystem to create: ext4 or btrfs" default:"ext4"`
	NoEncrypt  bool   `cli:"no-encrypt" help:"Skip LUKS2 encryption (not recommended)"`
	Force      bool   `cli:"force" help:"Do not prompt for confirmation before destroying data"`
	Password   string `cli:"password" help:"Repository passphrase (read from prompt if empty)"`
	NoActivate bool   `cli:"no-activate" help:"Do not mark the new repository as active"`
}

// Run executes the init command.
func (c *DeviceInitCmd) Run() error {
	if c.Device == "" {
		return fmt.Errorf("device argument is required")
	}
	if c.FS != "ext4" && c.FS != "btrfs" {
		return fmt.Errorf("--fs must be ext4 or btrfs")
	}
	encrypt := !c.NoEncrypt

	if !c.Force {
		fmt.Printf("\nThis will DESTROY all data on %s.\n", c.Device)
		ok, err := globalApp.CLI.ConfirmAction("Continue?", "y", "N", false)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("aborted")
		}
	}

	password := c.Password
	if encrypt && password == "" {
		p, err := readPassword("Repository passphrase: ")
		if err != nil {
			return err
		}
		confirm, err := readPassword("Repeat passphrase: ")
		if err != nil {
			return err
		}
		if p != confirm {
			return fmt.Errorf("passphrases do not match")
		}
		if p == "" {
			return fmt.Errorf("empty passphrase is not allowed")
		}
		password = p
	}

	repo, err := repoinit.InitRepository(repoinit.InitOptions{
		DevicePath: c.Device,
		Label:      c.Label,
		Encrypt:    encrypt,
		FS:         c.FS,
		Password:   password,
	})
	if err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	globalApp.Log.Term.Info().Msgf("Repository initialized at %s", repo.MountPath)

	if c.NoActivate {
		if err := repo.Close(); err != nil {
			return fmt.Errorf("failed to lock new repository: %w", err)
		}
		globalApp.Log.Term.Info().Msg("Repository created and locked (not activated).")
		return nil
	}

	if err := runtime.SetActive(runtime.ActiveState{
		DevicePath: repo.DevicePath,
		MountPath:  repo.MountPath,
		RepoPath:   repo.MountPath,
		LUKS:       repo.LUKS,
		Label:      repo.Marker.Label,
		UUID:       repo.Marker.UUID,
	}); err != nil {
		return fmt.Errorf("failed to persist active state: %w", err)
	}

	globalApp.Log.Term.Info().Msgf("Active repository set to %s (%s)", repo.DevicePath, repo.MountPath)
	return nil
}

// DeviceUnlockCmd implements `continuity device unlock`.
type DeviceUnlockCmd struct {
	cli.Base
	Device   string `arg:"" help:"Block device hosting a Continuity repository"`
	Password string `cli:"password" help:"LUKS passphrase (read from prompt if empty)"`
}

// Run executes the unlock command.
func (c *DeviceUnlockCmd) Run() error {
	if c.Device == "" {
		return fmt.Errorf("device argument is required")
	}

	password := c.Password
	if password == "" {
		needLUKS, err := isLUKSDevice(c.Device)
		if err != nil {
			return err
		}
		if needLUKS {
			p, err := readPassword("Repository passphrase: ")
			if err != nil {
				return err
			}
			password = p
		}
	}

	repo, err := repoinit.UnlockRepository(repoinit.UnlockOptions{
		DevicePath: c.Device,
		Password:   password,
	})
	if err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}

	if err := runtime.SetActive(runtime.ActiveState{
		DevicePath: repo.DevicePath,
		MountPath:  repo.MountPath,
		RepoPath:   repo.MountPath,
		LUKS:       repo.LUKS,
		Label:      repo.Marker.Label,
		UUID:       repo.Marker.UUID,
	}); err != nil {
		_ = repo.Close()
		return fmt.Errorf("failed to persist active state: %w", err)
	}

	globalApp.Log.Term.Info().Msgf("Repository unlocked at %s", repo.MountPath)
	globalApp.Log.Term.Info().Msgf("Active repository set to %s", repo.DevicePath)
	return nil
}

// DeviceLockCmd implements `continuity device lock`.
type DeviceLockCmd struct {
	cli.Base
	Device string `arg:"optional" help:"Block device to lock (defaults to the active repository)"`
}

// Run executes the lock command.
func (c *DeviceLockCmd) Run() error {
	device := c.Device
	if device == "" {
		active, err := runtime.GetActive()
		if err != nil {
			return err
		}
		if active == nil {
			return fmt.Errorf("no active repository to lock")
		}
		device = active.DevicePath
	}

	if err := repoinit.LockDevice(device); err != nil {
		return fmt.Errorf("lock failed: %w", err)
	}

	if active, err := runtime.GetActive(); err == nil && active != nil && active.DevicePath == device {
		if err := runtime.ClearActive(); err != nil {
			return fmt.Errorf("failed to clear active state: %w", err)
		}
	}

	globalApp.Log.Term.Info().Msgf("Repository on %s locked", device)
	return nil
}

// DeviceInfoCmd implements `continuity device info`.
type DeviceInfoCmd struct {
	cli.Base
	Device string `arg:"" help:"Block device path (e.g. /dev/sdb1)"`
}

// Run executes the info command.
func (c *DeviceInfoCmd) Run() error {
	if c.Device == "" {
		return fmt.Errorf("device argument is required")
	}
	devs, err := devices.List(devices.ListOptions{All: true})
	if err != nil {
		return err
	}
	var found *devices.Device
	for i := range devs {
		if devs[i].Path == c.Device {
			found = &devs[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("device not found: %s", c.Device)
	}

	rows := [][]string{
		{"Path", found.Path},
		{"Type", found.Type},
		{"Size", humanSize(found.Size)},
		{"Filesystem", fsLabel(*found)},
		{"Label", emptyDash(found.Label)},
		{"UUID", emptyDash(found.UUID)},
		{"Bus", transportLabel(*found)},
		{"Removable", boolLabel(found.Removable)},
		{"Mountpoint", mountLabel(*found)},
		{"Model", emptyDash(found.Model)},
		{"Continuity", repoLabel(*found)},
	}
	if found.ContinuityInfo != nil {
		rows = append(rows,
			[]string{"Repository Label", emptyDash(found.ContinuityInfo.Label)},
			[]string{"Repository UUID", emptyDash(found.ContinuityInfo.UUID)},
			[]string{"Created At", emptyDash(found.ContinuityInfo.CreatedAt)},
		)
	}

	fmt.Printf("\nDevice %s\n", found.Path)
	return globalApp.CLI.Table([]string{"Field", "Value"}, rows)
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func boolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func humanSize(b uint64) string {
	const k = 1024
	if b < k {
		return fmt.Sprintf("%d B", b)
	}
	v := float64(b) / k
	for _, u := range []string{"KiB", "MiB", "GiB", "TiB", "PiB"} {
		if v < k {
			return fmt.Sprintf("%.1f %s", v, u)
		}
		v /= k
	}
	return fmt.Sprintf("%.1f EiB", v)
}

func fsLabel(d devices.Device) string {
	if d.IsLUKS {
		return "LUKS"
	}
	if d.FSType == "" {
		return "-"
	}
	if d.Label != "" {
		return fmt.Sprintf("%s (%s)", d.FSType, d.Label)
	}
	return d.FSType
}

func transportLabel(d devices.Device) string {
	if d.Transport == "" {
		return "-"
	}
	if d.Removable {
		return d.Transport + " (removable)"
	}
	return d.Transport
}

func mountLabel(d devices.Device) string {
	if d.Mountpoint == "" {
		return "-"
	}
	return d.Mountpoint
}

func repoLabel(d devices.Device) string {
	if d.IsContinuity {
		if d.ContinuityInfo != nil && d.ContinuityInfo.Label != "" {
			return "yes (" + d.ContinuityInfo.Label + ")"
		}
		return "yes"
	}
	if d.IsLUKS {
		return "locked LUKS"
	}
	return "no"
}

// readPassword reads a passphrase from the terminal with echo disabled,
// falling back to plain stdin when the process isn't attached to a TTY.
func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(syscall.Stdin)
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	var line string
	_, err := fmt.Fscanln(os.Stdin, &line)
	return line, err
}

func isLUKSDevice(devicePath string) (bool, error) {
	devs, err := devices.List(devices.ListOptions{All: true})
	if err != nil {
		return false, err
	}
	for _, d := range devs {
		if d.Path == devicePath {
			return d.IsLUKS, nil
		}
	}
	return false, fmt.Errorf("device not found: %s", devicePath)
}
