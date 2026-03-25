package storage

/*License: GPLv3
Authors:
Vanilla OS Contributors <https://github.com/vanilla-os/>
Copyright: 2026
Description: SFTP remote storage backend.
*/

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"github.com/vanilla-os/continuity/pkg/v1/config"
	"golang.org/x/crypto/ssh"
)

// SFTPBackend provides backup storage over SSH/SFTP.
// All writes go directly to the remote host without local staging.
type SFTPBackend struct {
	cfg      *config.Config
	sshConn  *ssh.Client
	client   *sftp.Client
	basePath string
}

// NewSFTPBackend creates an SFTPBackend from config.
func NewSFTPBackend(cfg *config.Config) *SFTPBackend {
	return &SFTPBackend{
		cfg:      cfg,
		basePath: cfg.Remote.Path,
	}
}

// Connect establishes the SSH connection and creates the SFTP client.
func (b *SFTPBackend) Connect() error {
	r := b.cfg.Remote

	var authMethods []ssh.AuthMethod

	if r.KeyFile != "" {
		keyBytes, err := os.ReadFile(r.KeyFile)
		if err != nil {
			return fmt.Errorf("sftp: failed to read key file %s: %w", r.KeyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("sftp: failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if r.Password != "" {
		authMethods = append(authMethods, ssh.Password(r.Password))
	}

	port := r.Port
	if port == 0 {
		port = 22
	}

	sshCfg := &ssh.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", r.Host, port)
	conn, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return fmt.Errorf("sftp: failed to connect to %s: %w", addr, err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("sftp: failed to create client: %w", err)
	}

	b.sshConn = conn
	b.client = client
	return nil
}

// Close closes the SFTP client and SSH connection.
func (b *SFTPBackend) Close() error {
	if b.client != nil {
		b.client.Close()
	}
	if b.sshConn != nil {
		b.sshConn.Close()
	}
	return nil
}

func (b *SFTPBackend) ReadFile(path string) ([]byte, error) {
	f, err := b.client.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (b *SFTPBackend) WriteFile(path string, data []byte, perm os.FileMode) error {
	f, err := b.client.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(perm); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func (b *SFTPBackend) MkdirAll(path string, _ os.FileMode) error {
	return b.client.MkdirAll(path)
}

func (b *SFTPBackend) Remove(path string) error {
	return b.client.Remove(path)
}

func (b *SFTPBackend) RemoveAll(path string) error {
	return b.client.RemoveAll(path)
}

// sftpDirEntry wraps sftp.FileInfo to implement os.DirEntry.
type sftpDirEntry struct {
	info os.FileInfo
}

func (e *sftpDirEntry) Name() string               { return e.info.Name() }
func (e *sftpDirEntry) IsDir() bool                { return e.info.IsDir() }
func (e *sftpDirEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e *sftpDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

func (b *SFTPBackend) ReadDir(path string) ([]os.DirEntry, error) {
	infos, err := b.client.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = &sftpDirEntry{info: info}
	}
	return entries, nil
}

func (b *SFTPBackend) Stat(path string) (os.FileInfo, error) {
	return b.client.Stat(path)
}

// Walk traverses the remote directory tree, calling fn for each entry.
func (b *SFTPBackend) Walk(root string, fn filepath.WalkFunc) error {
	return b.sftpWalk(root, root, fn)
}

func (b *SFTPBackend) sftpWalk(root, current string, fn filepath.WalkFunc) error {
	info, err := b.client.Stat(current)
	if err != nil {
		return fn(current, nil, err)
	}

	if err := fn(current, info, nil); err != nil {
		if current == root || !info.IsDir() {
			return err
		}
		if err == filepath.SkipDir {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	entries, err := b.client.ReadDir(current)
	if err != nil {
		return fn(current, info, err)
	}

	for _, entry := range entries {
		child := filepath.Join(current, entry.Name())
		if err := b.sftpWalk(root, child, fn); err != nil {
			if err == filepath.SkipDir {
				continue
			}
			return err
		}
	}
	return nil
}

func (b *SFTPBackend) Rename(oldPath, newPath string) error {
	// Ensure parent directory exists for the destination
	if err := b.client.MkdirAll(filepath.Dir(newPath)); err != nil {
		return err
	}
	return b.client.PosixRename(oldPath, newPath)
}

// CopyFromNative uploads nativeSrc (local path) directly to backendDst (remote path).
func (b *SFTPBackend) CopyFromNative(nativeSrc, backendDst string) error {
	// Resolve symlinks on the root so filepath.Walk can descend into it.
	// (filepath.Walk does not follow symlinks, including the root itself.)
	resolvedSrc, err := filepath.EvalSymlinks(nativeSrc)
	if err != nil {
		return fmt.Errorf("failed to resolve path %s: %w", nativeSrc, err)
	}

	err = filepath.Walk(resolvedSrc, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "\r\033[K  ⚠ skipped %s: %v\n", localPath, err)
			return nil
		}

		relPath, err := filepath.Rel(resolvedSrc, localPath)
		if err != nil {
			return err
		}
		remotePath := filepath.Join(backendDst, relPath)

		if info.IsDir() {
			return b.client.MkdirAll(remotePath)
		}

		// Skip non-regular files: sockets, devices, named pipes, symlinks.
		if !info.Mode().IsRegular() {
			return nil
		}

		fmt.Fprintf(os.Stderr, "\r\033[K  → %s", localPath)

		if err := b.client.MkdirAll(filepath.Dir(remotePath)); err != nil {
			return err
		}

		src, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := b.client.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
	fmt.Fprintf(os.Stderr, "\r\033[K")
	return err
}

// CopyToNative downloads backendSrc (remote path) to nativeDst (local path).
func (b *SFTPBackend) CopyToNative(backendSrc, nativeDst string) error {
	return b.sftpWalk(backendSrc, backendSrc, func(remotePath string, info os.FileInfo, err error) error {
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

		src, err := b.client.Open(remotePath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

func (b *SFTPBackend) BasePath() string          { return b.basePath }
func (b *SFTPBackend) IsLocal() bool             { return false }
func (b *SFTPBackend) SupportsDeduplicate() bool { return false }
