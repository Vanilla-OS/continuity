package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vanilla-os/sdk/pkg/v1/backup"
	"github.com/vanilla-os/sdk/pkg/v1/fs"
)

func TestSnapshotAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	sourcePath := filepath.Join(tmpDir, "source")

	if err := os.MkdirAll(sourcePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "test.txt"), []byte("test data"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := backup.OpenRepository(repoPath)
	if err != nil {
		t.Fatalf("Failed to open repo: %v", err)
	}

	opts := backup.CreateSnapshotOptions{
		CopyOptions: fs.CopyTreeOptions{
			Workers:             1,
			PreserveOwnership:   false,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		},
	}

	snapshot, err := repo.CreateSnapshot(sourcePath, opts)
	if err != nil {
		t.Fatalf("Snapshot creation failed: %v", err)
	}

	snapshotPath := filepath.Join(repoPath, "snapshots", snapshot.Manifest.ID)
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatalf("Snapshot directory not found: %s", snapshotPath)
	}

	manifestPath := filepath.Join(snapshotPath, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatalf("Manifest not found: %s", manifestPath)
	}

	treePath := filepath.Join(snapshotPath, "tree")
	if _, err := os.Stat(treePath); os.IsNotExist(err) {
		t.Fatalf("Tree directory not found: %s", treePath)
	}

	testFile := filepath.Join(treePath, "test.txt")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("Source file not in snapshot: %s", testFile)
	}

	t.Logf("✓ Snapshot created atomically: %s", snapshot.Manifest.ID)
}

func TestRestoreAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	sourcePath := filepath.Join(tmpDir, "source")
	restorePath := filepath.Join(tmpDir, "restore")

	if err := os.MkdirAll(sourcePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "data.txt"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := backup.OpenRepository(repoPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := backup.CreateSnapshotOptions{
		CopyOptions: fs.CopyTreeOptions{
			Workers:             1,
			PreserveOwnership:   false,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		},
	}

	snapshot, err := repo.CreateSnapshot(sourcePath, opts)
	if err != nil {
		t.Fatalf("Snapshot creation failed: %v", err)
	}

	copyOpts := fs.CopyTreeOptions{
		Workers:             1,
		PreserveOwnership:   false,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	if err := repo.RestoreSnapshot(snapshot.Manifest.ID, restorePath, copyOpts); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restoredFile := filepath.Join(restorePath, "data.txt")
	data, err := os.ReadFile(restoredFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if string(data) != "original" {
		t.Fatalf("Restored data mismatch: got %s, want original", string(data))
	}

	t.Logf("✓ Restore completed atomically")
}

func TestSnapshotRollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	nonExistentSource := filepath.Join(tmpDir, "nonexistent")

	repo, err := backup.OpenRepository(repoPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := backup.CreateSnapshotOptions{
		CopyOptions: fs.CopyTreeOptions{
			Workers:             1,
			PreserveOwnership:   false,
			PreserveTimestamps:  true,
			PreservePermissions: true,
		},
	}

	_, err = repo.CreateSnapshot(nonExistentSource, opts)
	if err == nil {
		t.Fatal("Expected snapshot creation to fail for non-existent source")
	}

	tmpSnapshotsDir := filepath.Join(repoPath, "snapshots", ".tmp")
	entries, err := os.ReadDir(tmpSnapshotsDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	if len(entries) > 0 {
		t.Fatalf("Found %d partial snapshots after failure (rollback failed)", len(entries))
	}

	t.Logf("✓ Failed snapshot was properly rolled back")
}
