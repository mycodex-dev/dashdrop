package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestSaveUploadGeneratesThumbnail(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, "/d")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	html := "<!doctype html><html><body><h1>Hi</h1></body></html>"
	entry, err := store.SaveUpload("sales-report.html", strings.NewReader(html), int64(len(html)), nil, 1<<20, 1<<20)
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}
	if entry.Title != "sales-report" {
		t.Fatalf("title = %q", entry.Title)
	}

	thumbPath, err := store.ThumbPath(entry.Slug)
	if err != nil {
		t.Fatalf("ThumbPath: %v", err)
	}
	info, err := os.Stat(thumbPath)
	if err != nil {
		t.Fatalf("stat thumb: %v", err)
	}
	if info.Size() < 100 {
		t.Fatalf("thumb too small: %d", info.Size())
	}

	data, err := os.ReadFile(thumbPath)
	if err != nil {
		t.Fatalf("read thumb: %v", err)
	}
	if data[0] != 0x89 || string(data[1:4]) != "PNG" {
		t.Fatal("generated thumb is not a PNG")
	}
}

func TestGenerateThumbnail(t *testing.T) {
	data, err := GenerateThumbnail("Hello Dashboard")
	if err != nil {
		t.Fatalf("GenerateThumbnail: %v", err)
	}
	if len(data) < 100 {
		t.Fatalf("png too small: %d", len(data))
	}
	if data[0] != 0x89 || string(data[1:4]) != "PNG" {
		t.Fatal("not a PNG")
	}
}
