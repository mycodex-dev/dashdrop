package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadManifestRefreshesPublicURLs(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, "/drop")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	stale := []DashboardEntry{{
		Slug:      "abc123",
		Title:     "Old Prefix",
		CreatedAt: now,
		UpdatedAt: now,
		ThumbURL:  "/api/dashboards/abc123/thumb.png",
		URL:       "/d/abc123",
	}}
	data, err := json.MarshalIndent(stale, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	entries, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List len = %d, want 1", len(entries))
	}
	if got := entries[0].URL; got != "/drop/abc123" {
		t.Fatalf("URL = %q, want %q", got, "/drop/abc123")
	}
	if got := entries[0].ThumbURL; got != "/api/dashboards/abc123/thumb.png" {
		t.Fatalf("ThumbURL = %q", got)
	}
}
