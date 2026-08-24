<div align="center">
  <h1 align="center">Vanilla Continuity</h1>
  <p align="center">Vanilla Continuity provides snapshot-based backup and restore for Vanilla OS systems, including user data, Flatpak applications, and ABRoot metadata.</p>
</div>

## Help output

```md
Vanilla Continuity provides snapshot-based backup and restore for Vanilla OS

Usage:
  continuity [command]

Available Commands:
  backup          Create a new backup
  daemon          Start DBus daemon
  device          Manage block devices used as Continuity repositories
  help            Help about any command
  inspect         Inspect a backup snapshot
  list            List all backups
  man             Generate man page
  prune           Prune old backups
  restore         Restore from a backup
  status          Show Continuity status
  version         Show version information

Flags:
  -h, --help      help for continuity
      --version   version for continuity

Use "continuity [command] --help" for more information about a command.
```

## Installation

Vanilla Continuity is a single binary, which can be placed anywhere on the
system. Backup operations require administrative privileges. Configuration is
optional: built-in defaults are used when no configuration file is found. A
configuration file can be placed in one of these locations, ordered by
priority:

- `~/.config/continuity/config.json` -> for user configuration
- `./conf/continuity/config.json` -> for development purposes only
- `/etc/continuity/config.json` -> for administrative configuration
- `/usr/share/continuity/config.json` -> for system-wide configuration
- `/app/share/continuity/config.json` -> for flatpak configuration

The configuration file is a JSON file with the following structure:

```json
{
  "repository_path": "/var/lib/vanilla-continuity/repo",
  "default_deduplicate": false,
  "max_parallel_workers": 2,
  "retention_keep_last": 7,
  "exclude_patterns": [
    ".cache",
    ".local/share/Trash",
    "node_modules",
    ".tmp",
    "*.tmp"
  ],
  "enabled_providers": [
    "userdata",
    "flatpak",
    "abroot"
  ]
}
```

The following table describes each of the configuration options:

| Option | Description |
| --- | --- |
| `repository_path` | The location where backup snapshots are stored. |
| `default_deduplicate` | If set to `true`, Continuity will use deduplication when creating backups. |
| `max_parallel_workers` | The maximum number of parallel workers to use during backup and restore operations. |
| `retention_keep_last` | The number of snapshots to keep. Older snapshots are automatically pruned. If set to `0`, no automatic pruning occurs. |
| `exclude_patterns` | Glob patterns to exclude from backups. |
| `enabled_providers` | List of providers to enable. Valid values: `userdata`, `flatpak`, `abroot`. All enabled by default. |

## How it works

Vanilla Continuity creates snapshots of the system's user data, applications,
and ABRoot metadata. Each snapshot can be restored independently.

### Terminology

- **snapshot** - a snapshot is a point-in-time copy of the system's data, stored in the repository.
- **repository** - the repository is the location where all snapshots are stored. It can be a local directory or an encrypted USB device.
- **provider** - a provider is responsible for backing up and restoring a specific type of data (e.g., user data, Flatpak apps, ABRoot metadata).

### Backup process

The backup process is composed of multiple providers, each responsible for a specific type of data:

- **UserData** - backs up all user home directories from `/home/*`.
- **Flatpak** - backs up the list of installed Flatpak applications.
- **ABRoot** - backs up ABRoot metadata from `/etc/abroot`.

You can selectively enable/disable providers via the `enabled_providers` configuration field. This is useful if you want to exclude large directories (e.g., 500GB `/home` data) or only backup specific components.

Each provider runs independently, and the results are collected into a single snapshot. The snapshot is then stored in the repository with a unique ID and timestamp.

### Restore process

The restore process reads a snapshot from the repository and applies it to the system. Each provider is responsible for restoring its own data:

- **UserData** - restores user home directories to `/home/*`.
- **Flatpak** - reinstalls Flatpak applications from the backup list.
- **ABRoot** - restores ABRoot metadata to `/etc/abroot` and triggers `abroot pkg sync`.

Providers are restored independently. Continuity reports provider failures and
continues with the remaining providers, so inspect the command output before
rebooting or relying on the restored state.

### Retention pruning

When `retention_keep_last` is set to a value greater than `0`, Continuity will automatically prune old snapshots after each backup. Only the most recent snapshots are kept.

Manual pruning is also available:

```bash
continuity prune --keep-last 5
```

### Dry-run mode

Continuity supports dry-run mode, which simulates backup and restore operations without making any changes to the system:

```bash
continuity backup test-label --dry-run
continuity restore <snapshot-id> --dry-run
```

### Inspecting backups

View detailed information about your backups:

```bash
# List all backups
continuity list

# List with detailed information (size, providers, deduplication)
continuity list --details

# Inspect a specific snapshot
continuity inspect <snapshot-id>
```

The `inspect` command shows:
- Snapshot ID and creation date
- Total size (deduplicated)
- Providers included (UserData, Flatpak, ABRoot)
- Source path and deduplication status
- **Provider content**:
  - **Flatpak**: List of installed apps with IDs
  - **ABRoot**: Files backed up from /etc/abroot
  - **UserData**: User home directories with sizes

### External devices

Continuity can format an external block device as an ext4 or Btrfs repository.
LUKS2 encryption is enabled by default.

```bash
continuity device list
continuity device init /dev/sdb1 --label reunion-backups
continuity device unlock /dev/sdb1
continuity device info /dev/sdb1
continuity device lock /dev/sdb1
```

`device init` destroys existing data on the selected device. Check the device
path before confirming the operation.

### Remote repositories

Set the optional `remote` configuration object to store backups over SFTP,
FTP, SMB, or NFS. For example:

```json
{
  "remote": {
    "type": "sftp",
    "host": "backup.example.com",
    "port": 22,
    "user": "backup",
    "key_file": "/root/.ssh/continuity",
    "path": "/srv/backups/continuity"
  }
}
```

Use a protected configuration file and prefer an SFTP key over a stored
password. SMB requires `cifs-utils` on the host.

### DBus service

Continuity provides a DBus service for system integration. The service is available at `org.vanillaos.Continuity` and provides the following methods:

- `CreateBackup(label: string) -> (snapshot_id: string)`
- `ListBackups() -> (snapshot_ids: array[string])`
- `RestoreBackup(snapshot_id: string) -> (success: boolean)`
- `GetStatus() -> (status: string)`
- `ListDevices(include_all: boolean) -> (devices: array[dict])`
- `GetDeviceInfo(device_path: string) -> (info: dict)`
- `InitRepository(device_path, label, encrypt, fs, password, activate) -> (mount_path: string)`
- `UnlockRepository(device_path, password) -> (mount_path: string)`
- `LockRepository(device_path: string) -> (success: boolean)`
- `GetActiveRepository() -> (info: dict)`

To start the DBus daemon:

```bash
continuity daemon
```

Or install the systemd service:

```bash
sudo systemctl enable --now vanilla-continuity
```

## Building

Continuity requires Go 1.21+ and libudev-dev:

```bash
sudo apt install libudev-dev
make build
```

The resulting binary is placed in `bin/continuity`.

## Testing

Continuity includes unit and integration tests:

```bash
make test
```
