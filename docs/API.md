# Vanilla Continuity API Documentation

## DBus Interface

**Bus Name:** `org.vanillaos.Continuity`  
**Object Path:** `/org/vanillaos/Continuity`  
**Interface:** `org.vanillaos.Continuity`

### Methods

#### `CreateBackup(label: string) → (snapshot_id: string)`
Creates a new system backup with the specified label.

**Arguments:**
- `label` (string): Human-readable label for the backup (e.g., "before-upgrade")

**Returns:**
- `snapshot_id` (string): Unique ID of the created snapshot (e.g., "20260324T213045Z-abc123")

**Example:**
```bash
gdbus call --system --dest org.vanillaos.Continuity \
  --object-path /org/vanillaos/Continuity \
  --method org.vanillaos.Continuity.CreateBackup "manual-backup"
```

#### `ListBackups() → (snapshot_ids: array[string])`
Lists all available backups in the repository.

**Returns:**
- `snapshot_ids` (array[string]): Array of snapshot IDs

**Example:**
```bash
gdbus call --system --dest org.vanillaos.Continuity \
  --object-path /org/vanillaos/Continuity \
  --method org.vanillaos.Continuity.ListBackups
```

#### `RestoreBackup(snapshot_id: string) → (success: boolean)`
Restores the system from a backup snapshot.

**Arguments:**
- `snapshot_id` (string): ID of the snapshot to restore

**Returns:**
- `success` (boolean): True if restore succeeded

**Example:**
```bash
gdbus call --system --dest org.vanillaos.Continuity \
  --object-path /org/vanillaos/Continuity \
  --method org.vanillaos.Continuity.RestoreBackup "20260324T213045Z-abc123"
```

#### `GetStatus() → (status: string)`
Returns the current service status.

**Returns:**
- `status` (string): Service status ("ready", "busy", etc.)

**Example:**
```bash
gdbus call --system --dest org.vanillaos.Continuity \
  --object-path /org/vanillaos/Continuity \
  --method org.vanillaos.Continuity.GetStatus
```

## CLI Interface

### Commands

#### `continuity backup [label]`
Create a new backup.

**Arguments:**
- `label` (optional): Backup label (default: "manual")

**Example:**
```bash
sudo continuity backup before-upgrade
```

#### `continuity list`
List all available backups.

**Options:**
- `--details`: Show detailed information (size, providers, deduplication)

**Example:**
```bash
continuity list
continuity list --details
```

#### `continuity inspect <snapshot-id>`
Show detailed information about a specific backup snapshot.

**Arguments:**
- `snapshot-id` (required): ID of the snapshot to inspect

**Example:**
```bash
continuity inspect 20260325T094705Z-68b19f6b
```

**Output includes:**
- Snapshot ID and creation timestamp
- Total size (deduplicated)
- Source path
- Providers included (UserData, Flatpak, ABRoot)
- Deduplication status
- **Provider content details**:
  - **Flatpak**: List of backed up applications with app IDs
  - **ABRoot**: Files from /etc/abroot with sizes
  - **UserData**: User directories with total sizes

#### `continuity restore <snapshot-id>`
Restore from a backup snapshot.

**Arguments:**
- `snapshot-id` (required): ID of the snapshot to restore

**Example:**
```bash
sudo continuity restore 20260324T213045Z-abc123
```

#### `continuity daemon`
Start the DBus daemon service.

**Example:**
```bash
sudo continuity daemon
```

#### `continuity version`
Show version information.

**Example:**
```bash
continuity version
```

#### `continuity status`
Show Continuity status.

**Example:**
```bash
continuity status
```

## Configuration API

Configuration is managed via JSON at `/var/lib/vanilla-continuity/config.json`.

### Configuration Structure

```go
type Config struct {
    RepositoryPath     string   `json:"repository_path"`
    DefaultDeduplicate bool     `json:"default_deduplicate"`
    MaxParallelWorkers int      `json:"max_parallel_workers"`
    RetentionKeepLast  int      `json:"retention_keep_last"`
    ExcludePatterns    []string `json:"exclude_patterns"`
    EnabledProviders   []string `json:"enabled_providers"`
}
```

### Example Configuration

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

### Provider Configuration

The `enabled_providers` field allows selective enablement of backup providers:

- **userdata**: Backs up `/home/*` directories
- **flatpak**: Backs up Flatpak application list
- **abroot**: Backs up ABRoot metadata from `/etc/abroot`

All providers are enabled by default. To exclude a provider (e.g., to skip backing up large `/home` data), remove it from the list:

```json
{
  "enabled_providers": [
    "flatpak",
    "abroot"
  ]
}
```

## Provider API

Continuity uses a provider pattern for backup sources.

### Provider Interface

```go
type BackupProvider interface {
    Name() string
    Backup(app *app.App) (string, error)
    Restore(app *app.App, sourcePath string) error
}
```

### Built-in Providers

1. **UserDataProvider** — Backs up `/home/*`
2. **FlatpakProvider** — Backs up Flatpak application list
3. **ABRootProvider** — Backs up `/etc/abroot/*` metadata

## Repository API

The repository wrapper provides snapshot management.

### Repository Manager

```go
type Manager struct {
    App    *app.App
    Config *config.Config
    Repo   *backup.Repository
}

func NewManager(app *app.App, cfg *config.Config) (*Manager, error)
func (m *Manager) CreateSnapshot(source string, tags map[string]string) (string, error)
func (m *Manager) ListSnapshots() ([]backup.SnapshotManifest, error)
func (m *Manager) RestoreSnapshot(snapshotID, destination string) error
```

## Error Handling

All DBus methods return `*dbus.Error` on failure.  
All CLI commands return non-zero exit codes on failure.

**Common error scenarios:**
- Insufficient permissions (requires root for most operations)
- Repository not found/corrupted
- Snapshot ID not found
- Disk space exhausted
- Provider failure (e.g., Flatpak not installed)

## Logging

Logs are written to:
- **Terminal output** (when run interactively)
- **Systemd journal** (when run as daemon)

Log levels: INFO, WARN, ERROR

**Example:**
```bash
# View daemon logs
journalctl -u vanilla-continuity -f
```
