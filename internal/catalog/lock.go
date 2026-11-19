// Package catalog downloads, validates, and caches the models.dev catalog.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockFileName = "catalog.lock.json"

// Lock records metadata about a successfully synced catalog.
type Lock struct {
	SourceURL string    `json:"source_url"`
	SyncedAt  time.Time `json:"synced_at"`
	SHA256    string    `json:"sha256"`
	Bytes     int64     `json:"bytes"`
	TTLHours  int       `json:"ttl_hours"`
}

// Expired reports whether the lock is older than its TTL relative to now.
// A non-positive TTL is treated as already expired.
func (l *Lock) Expired(now time.Time) bool {
	if l == nil || l.TTLHours <= 0 {
		return true
	}
	return now.After(l.SyncedAt.Add(time.Duration(l.TTLHours) * time.Hour))
}

// WriteLock writes lock to destDir/catalog.lock.json atomically. The
// destination directory is created if it does not already exist.
func WriteLock(destDir string, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("lock is nil")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}
	data = append(data, '\n')

	// Unique temp file per writer; see the catalog write for why.
	tmpFile, err := os.CreateTemp(destDir, "catalog.lock.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create lock temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("write lock temp file: %w", err)
	}
	if err := tmpFile.Chmod(0644); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("chmod lock temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return fmt.Errorf("sync lock temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close lock temp file: %w", err)
	}
	finalPath := filepath.Join(destDir, lockFileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("rename lock file: %w", err)
	}
	return nil
}

// ReadLock reads lock from destDir/catalog.lock.json.
func ReadLock(destDir string) (*Lock, error) {
	path := filepath.Join(destDir, lockFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}
	return &lock, nil
}
