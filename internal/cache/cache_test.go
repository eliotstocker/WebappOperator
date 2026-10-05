package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "webapp-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr, err := NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to init disk manager: %v", err)
	}

	ns := "default"
	web := "my-app"
	rev := "v1-0-0"

	if mgr.IsCached(ns, web, rev) {
		t.Fatalf("expected not cached initially")
	}

	// 1. Ensure revision with dummy unpack
	dir, err := mgr.EnsureRevision(context.Background(), ns, web, rev, func(destDir string) error {
		return os.WriteFile(filepath.Join(destDir, "index.html"), []byte("<h1>Hello</h1>"), 0644)
	})
	if err != nil {
		t.Fatalf("unexpected EnsureRevision error: %v", err)
	}

	if !mgr.IsCached(ns, web, rev) {
		t.Fatalf("expected revision to be cached after ensure")
	}

	// 2. Call EnsureRevision again when already cached (should return immediately)
	dir2, err := mgr.EnsureRevision(context.Background(), ns, web, rev, func(destDir string) error {
		t.Fatal("fetchFn should not be called when already cached")
		return nil
	})
	if err != nil || dir2 != dir {
		t.Errorf("expected cached dir returned without error")
	}

	// 3. Verify file content
	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || string(content) != "<h1>Hello</h1>" {
		t.Fatalf("file content mismatch: %v, %s", err, string(content))
	}

	// 4. Test EnsureRevision with error in fetchFn
	_, err = mgr.EnsureRevision(context.Background(), ns, web, "failed-rev", func(destDir string) error {
		return fmt.Errorf("network timeout")
	})
	if err == nil {
		t.Fatalf("expected error when fetchFn fails")
	}

	// 5. Test Prune
	if err := mgr.PruneRevision(ns, web, rev); err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	if mgr.IsCached(ns, web, rev) {
		t.Fatalf("expected not cached after prune")
	}
}
