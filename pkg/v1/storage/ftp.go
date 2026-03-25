package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: FTP remote storage backend.
*/

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/vanilla-os/continuity/pkg/v1/config"
)

// copyTask holds the metadata for a single file to upload.
type copyTask struct {
	localPath  string
	remotePath string
	mode       os.FileMode
}

// clearProgress clears the current progress line.
func clearProgress() {
	fmt.Fprintf(os.Stderr, "\r\033[K")
}

// FTPBackend provides backup storage over FTP.
// All writes go directly to the remote host without local staging.
type FTPBackend struct {
	cfg      *config.Config
	conn     *ftp.ServerConn
	basePath string
}

// NewFTPBackend creates an FTPBackend from config.
func NewFTPBackend(cfg *config.Config) *FTPBackend {
	return &FTPBackend{
		cfg:      cfg,
		basePath: cfg.Remote.Path,
	}
}

// Connect dials the FTP server and authenticates.
func (b *FTPBackend) Connect() error {
	r := b.cfg.Remote

	port := r.Port
	if port == 0 {
		port = 21
	}

	addr := fmt.Sprintf("%s:%d", r.Host, port)
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return fmt.Errorf("ftp: failed to dial %s: %w", addr, err)
	}

	if err := conn.Login(r.User, r.Password); err != nil {
		conn.Quit()
		return fmt.Errorf("ftp: login failed: %w", err)
	}

	b.conn = conn
	return nil
}

// Close logs out and closes the FTP connection.
func (b *FTPBackend) Close() error {
	if b.conn != nil {
		return b.conn.Quit()
	}
	return nil
}

func (b *FTPBackend) ReadFile(path string) ([]byte, error) {
	resp, err := b.conn.Retr(path)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	return io.ReadAll(resp)
}

func (b *FTPBackend) WriteFile(path string, data []byte, _ os.FileMode) error {
	if err := b.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	r := &byteReader{data: data}
	return b.conn.Stor(path, r)
}

// byteReader implements io.Reader over a []byte.
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// MkdirAll creates remote directories recursively.
// Uses ChangeDir to check existence since List may fail with restricted permissions.
func (b *FTPBackend) MkdirAll(path string, _ os.FileMode) error {
	if path == "" || path == "." || path == "/" {
		return nil
	}

	// CWD is a lighter existence check that works even without LIST permissions.
	if err := b.conn.ChangeDir(path); err == nil {
		return nil
	}

	// Ensure parent exists first.
	if parent := filepath.Dir(path); parent != path {
		if err := b.MkdirAll(parent, 0755); err != nil {
			return err
		}
	}

	// Create directory; tolerate a race where it was just created.
	if err := b.conn.MakeDir(path); err != nil {
		if cdErr := b.conn.ChangeDir(path); cdErr == nil {
			return nil // already exists
		}
		return fmt.Errorf("ftp: MkdirAll %s: %w", path, err)
	}
	return nil
}

func (b *FTPBackend) Remove(path string) error {
	return b.conn.Delete(path)
}

func (b *FTPBackend) RemoveAll(path string) error {
	return b.conn.RemoveDirRecur(path)
}

// ftpDirEntry wraps an ftp.Entry to implement os.DirEntry.
type ftpDirEntry struct {
	entry *ftp.Entry
}

func (e *ftpDirEntry) Name() string      { return e.entry.Name }
func (e *ftpDirEntry) IsDir() bool       { return e.entry.Type == ftp.EntryTypeFolder }
func (e *ftpDirEntry) Type() fs.FileMode {
	if e.entry.Type == ftp.EntryTypeFolder {
		return fs.ModeDir
	}
	return 0
}

func (e *ftpDirEntry) Info() (fs.FileInfo, error) {
	return &ftpFileInfo{entry: e.entry}, nil
}

// ftpFileInfo wraps an ftp.Entry to implement os.FileInfo.
type ftpFileInfo struct {
	entry *ftp.Entry
}

func (f *ftpFileInfo) Name() string      { return f.entry.Name }
func (f *ftpFileInfo) Size() int64       { return int64(f.entry.Size) }
func (f *ftpFileInfo) Mode() os.FileMode {
	if f.entry.Type == ftp.EntryTypeFolder {
		return os.ModeDir | 0755
	}
	return 0644
}
func (f *ftpFileInfo) ModTime() time.Time { return f.entry.Time }
func (f *ftpFileInfo) IsDir() bool        { return f.entry.Type == ftp.EntryTypeFolder }
func (f *ftpFileInfo) Sys() interface{}   { return nil }

func (b *FTPBackend) ReadDir(path string) ([]os.DirEntry, error) {
	entries, err := b.conn.List(path)
	if err != nil {
		return nil, err
	}

	result := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		result = append(result, &ftpDirEntry{entry: e})
	}
	return result, nil
}

func (b *FTPBackend) Stat(path string) (os.FileInfo, error) {
	parent := filepath.Dir(path)
	name := filepath.Base(path)

	entries, err := b.conn.List(parent)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.Name == name {
			return &ftpFileInfo{entry: e}, nil
		}
	}
	return nil, fmt.Errorf("ftp: %s: no such file or directory", path)
}

// Walk traverses the remote directory tree via recursive ReadDir.
func (b *FTPBackend) Walk(root string, fn filepath.WalkFunc) error {
	return b.ftpWalk(root, fn)
}

func (b *FTPBackend) ftpWalk(current string, fn filepath.WalkFunc) error {
	info, err := b.Stat(current)
	if err != nil {
		return fn(current, nil, err)
	}

	if err := fn(current, info, nil); err != nil {
		if !info.IsDir() || err == filepath.SkipDir {
			return err
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	entries, err := b.ReadDir(current)
	if err != nil {
		return fn(current, info, err)
	}

	for _, entry := range entries {
		child := filepath.Join(current, entry.Name())
		if err := b.ftpWalk(child, fn); err != nil {
			if err == filepath.SkipDir {
				continue
			}
			return err
		}
	}
	return nil
}

func (b *FTPBackend) Rename(oldPath, newPath string) error {
	if err := b.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}
	return b.conn.Rename(oldPath, newPath)
}

// newConn dials a fresh FTP connection authenticated with the same credentials.
func (b *FTPBackend) newConn() (*ftp.ServerConn, error) {
	r := b.cfg.Remote
	port := r.Port
	if port == 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", r.Host, port)
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("ftp: failed to dial %s: %w", addr, err)
	}
	if err := conn.Login(r.User, r.Password); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("ftp: login failed: %w", err)
	}
	return conn, nil
}

// CopyFromNative uploads nativeSrc (local path) directly to backendDst (remote path).
func (b *FTPBackend) CopyFromNative(nativeSrc, backendDst string, excludePatterns []string) error {
	// Resolve symlinks on the root so filepath.Walk can descend into it.
	// (filepath.Walk does not follow symlinks, including the root itself.)
	resolvedSrc, err := filepath.EvalSymlinks(nativeSrc)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", nativeSrc, err)
	}

	// Phase 1: Walk and collect file tasks and directories.
	var tasks []copyTask
	var dirs []string

	err = filepath.Walk(resolvedSrc, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r\033[K  ⚠ skipped %s: %v\n", localPath, err)
			return nil
		}

		relPath, err := filepath.Rel(resolvedSrc, localPath)
		if err != nil {
			return err
		}

		if relPath != "." && shouldExclude(relPath, excludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		remotePath := filepath.Join(backendDst, relPath)

		if info.IsDir() {
			dirs = append(dirs, remotePath)
			return nil
		}

		// Skip non-regular files: sockets, devices, named pipes, symlinks.
		if !info.Mode().IsRegular() {
			return nil
		}

		tasks = append(tasks, copyTask{
			localPath:  localPath,
			remotePath: remotePath,
			mode:       info.Mode(),
		})
		return nil
	})
	if err != nil {
		return err
	}

	// Phase 2: Create all directories sequentially on main connection (parent-first order).
	for _, dir := range dirs {
		if err := b.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Phase 3: Parallel upload.
	workers := b.cfg.MaxParallelWorkers
	if workers < 1 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskCh := make(chan copyTask)
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	var uploaded int64
	total := len(tasks)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := b.newConn()
			if err != nil {
				errCh <- err
				cancel()
				return
			}
			defer conn.Quit()

			for task := range taskCh {
				if ctx.Err() != nil {
					return
				}

				f, err := os.Open(task.localPath)
				if err != nil {
					errCh <- err
					cancel()
					return
				}
				err = conn.Stor(task.remotePath, f)
				f.Close()
				if err != nil {
					errCh <- err
					cancel()
					return
				}

				n := atomic.AddInt64(&uploaded, 1)
				fmt.Fprintf(os.Stderr, "\r\033[2K  → %d/%d files", n, total)
			}
		}()
	}

	for _, task := range tasks {
		select {
		case taskCh <- task:
		case <-ctx.Done():
		}
	}
	close(taskCh)

	wg.Wait()
	clearProgress()

	close(errCh)
	for e := range errCh {
		if e != nil {
			return e
		}
	}
	return nil
}

// CopyToNative downloads backendSrc (remote path) to nativeDst (local path).
func (b *FTPBackend) CopyToNative(backendSrc, nativeDst string) error {
	return b.ftpWalk(backendSrc, func(remotePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, relErr := filepath.Rel(backendSrc, remotePath)
		if relErr != nil {
			return relErr
		}
		localPath := filepath.Join(nativeDst, relPath)

		if info.IsDir() {
			return os.MkdirAll(localPath, info.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}

		resp, err := b.conn.Retr(remotePath)
		if err != nil {
			return err
		}
		defer resp.Close()

		f, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, resp)
		return err
	})
}

func (b *FTPBackend) BasePath() string          { return b.basePath }
func (b *FTPBackend) IsLocal() bool             { return false }
func (b *FTPBackend) SupportsDeduplicate() bool { return false }
