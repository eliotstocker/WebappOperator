// Package cache manages local disk (emptyDir) caching of unpacked OCI assets.
package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sync/singleflight"
)

// Manager handles on-disk asset storage, single-flight fetching, and pruning.
type Manager interface {
	// RevisionDir returns the absolute path on disk where a specific revision is unpacked.
	RevisionDir(namespace, websiteName, revision string) string

	// IsCached checks whether the given revision directory exists and contains assets.
	IsCached(namespace, websiteName, revision string) bool

	// EnsureRevision executes fetchFn via a single-flight group to avoid duplicate concurrent pulls.
	EnsureRevision(ctx context.Context, namespace, websiteName, revision string, fetchFn func(destDir string) error) (string, error)

	// PruneRevision deletes an unpacked revision from local disk.
	PruneRevision(namespace, websiteName, revision string) error
}

type diskManager struct {
	baseDir string
	sfg     singleflight.Group
}

// NewDiskManager initializes a local disk cache manager rooted at baseDir.
func NewDiskManager(baseDir string) (Manager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base cache directory %q: %w", baseDir, err)
	}
	return &diskManager{
		baseDir: baseDir,
	}, nil
}

func (m *diskManager) RevisionDir(namespace, websiteName, revision string) string {
	return filepath.Join(m.baseDir, namespace, websiteName, revision)
}

func (m *diskManager) IsCached(namespace, websiteName, revision string) bool {
	entries, err := os.ReadDir(m.RevisionDir(namespace, websiteName, revision))
	return err == nil && len(entries) > 0
}

func (m *diskManager) EnsureRevision(ctx context.Context, namespace, websiteName, revision string, fetchFn func(destDir string) error) (string, error) {
	dir := m.RevisionDir(namespace, websiteName, revision)
	if m.IsCached(namespace, websiteName, revision) {
		return dir, nil
	}

	key := fmt.Sprintf("%s/%s/%s", namespace, websiteName, revision)
	_, err, _ := m.sfg.Do(key, func() (interface{}, error) {
		// Double check after acquiring singleflight lock
		if m.IsCached(namespace, websiteName, revision) {
			return dir, nil
		}

		tmpDir := fmt.Sprintf("%s.tmp-%d", dir, os.Getpid())
		_ = os.RemoveAll(tmpDir)
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create temp unpack dir: %w", err)
		}

		if err := fetchFn(tmpDir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("failed to fetch and unpack OCI layer: %w", err)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("failed to create parent directory for revision: %w", err)
		}

		// Atomic rename
		if err := os.Rename(tmpDir, dir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("failed to move unpacked revision into place: %w", err)
		}

		return dir, nil
	})

	if err != nil {
		return "", err
	}
	return dir, nil
}

func (m *diskManager) PruneRevision(namespace, websiteName, revision string) error {
	dir := m.RevisionDir(namespace, websiteName, revision)
	return os.RemoveAll(dir)
}
