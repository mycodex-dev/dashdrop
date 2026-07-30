package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mycodex-dev/dashdrop/internal/config"
	"github.com/mycodex-dev/dashdrop/internal/storage"
)

func TestHandleRobotsDisallowsPublicPrefix(t *testing.T) {
	h := &Handler{cfg: config.Config{PublicPathPrefix: "/drop"}}
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	h.HandleRobots(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"User-agent: *\n",
		"Disallow: /drop/\n",
		"Disallow: /library\n",
		"Disallow: /api/\n",
		"User-agent: GPTBot\n",
		"User-agent: Google-Extended\n",
		"User-agent: ClaudeBot\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt missing %q\nbody:\n%s", want, body)
		}
	}

	// Specific User-agent groups replace (not inherit) the * group, so each AI
	// group must disallow discovery endpoints as well as the public prefix.
	for _, ua := range []string{"GPTBot", "Google-Extended", "ClaudeBot"} {
		group := "User-agent: " + ua + "\nDisallow: /drop/\nDisallow: /library\nDisallow: /api/\n"
		if !strings.Contains(body, group) {
			t.Errorf("AI group %q missing full Disallow set\nbody:\n%s", ua, body)
		}
	}
}

func TestHandleServeSetsXRobotsTag(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir, "/d")
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	htmlBody := "<html><body>hello</body></html>"
	html := strings.NewReader(htmlBody)
	thumb := bytes.NewReader(minimalPNG())
	entry, err := store.SaveUpload("hello.html", html, int64(len(htmlBody)), thumb, 1<<20, 1<<20)
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}

	h := New(store, config.Config{PublicPathPrefix: "/d", MaxUploadBytes: 1 << 20, MaxThumbBytes: 1 << 20}, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/d/"+entry.Slug, nil)
	req.SetPathValue("slug", entry.Slug)
	rec := httptest.NewRecorder()
	h.HandleServe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != robotsTagNoIndex {
		t.Fatalf("X-Robots-Tag = %q, want %q", got, robotsTagNoIndex)
	}
}

func TestHandleDownloadSetsXRobotsTag(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir, "/d")
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	htmlBody := "<html><body>hello</body></html>"
	html := strings.NewReader(htmlBody)
	thumb := bytes.NewReader(minimalPNG())
	entry, err := store.SaveUpload("hello.html", html, int64(len(htmlBody)), thumb, 1<<20, 1<<20)
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}

	h := New(store, config.Config{PublicPathPrefix: "/d", MaxUploadBytes: 1 << 20, MaxThumbBytes: 1 << 20}, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/api/dashboards/"+entry.Slug+"/download", nil)
	req.SetPathValue("slug", entry.Slug)
	rec := httptest.NewRecorder()
	h.HandleDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != robotsTagNoIndex {
		t.Fatalf("X-Robots-Tag = %q, want %q", got, robotsTagNoIndex)
	}
}

func minimalPNG() []byte {
	// 1x1 transparent PNG
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
