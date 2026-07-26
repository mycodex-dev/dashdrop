package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycodex-dev/dashdrop/internal/config"
	"github.com/mycodex-dev/dashdrop/internal/storage"
)

func testServer(t *testing.T) (*Server, *storage.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir, "/d")
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	cfg := config.Config{
		DataDir:          dir,
		MaxUploadBytes:   1 << 20,
		MaxThumbBytes:    1 << 20,
		PublicPathPrefix: "/d",
		BaseURL:          "http://dash.example.com",
		MCPEnabled:       true,
	}
	return New(store, cfg), store
}

func rpcPost(t *testing.T, h http.Handler, body any, sessionID string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set(headerSessionID, sessionID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeRPC(t *testing.T, resp *http.Response) jsonRPCResponse {
	t.Helper()
	defer resp.Body.Close()
	var out jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestInitializeAndUploadDashboard(t *testing.T) {
	srv, store := testServer(t)
	h := srv.Handler()

	initResp := rpcPost(t, h, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "0.1"},
		},
	}, "")
	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d", initResp.StatusCode)
	}
	sessionID := initResp.Header.Get(headerSessionID)
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id")
	}
	initBody := decodeRPC(t, initResp)
	if initBody.Error != nil {
		t.Fatalf("initialize error: %+v", initBody.Error)
	}

	notif := rpcPost(t, h, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}, sessionID)
	if notif.StatusCode != http.StatusAccepted {
		t.Fatalf("initialized status = %d, want 202", notif.StatusCode)
	}
	notif.Body.Close()

	listResp := rpcPost(t, h, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}, sessionID)
	listBody := decodeRPC(t, listResp)
	if listBody.Error != nil {
		t.Fatalf("tools/list error: %+v", listBody.Error)
	}

	uploadResp := rpcPost(t, h, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "upload_dashboard",
			"arguments": map[string]any{
				"html":     "<!doctype html><html><body><h1>Hello MCP</h1></body></html>",
				"filename": "hello-mcp.html",
				"title":    "Hello MCP",
				"slug":     "hello-mcp",
				"tags":     []string{"agent", "demo"},
			},
		},
	}, sessionID)
	uploadBody := decodeRPC(t, uploadResp)
	if uploadBody.Error != nil {
		t.Fatalf("tools/call error: %+v", uploadBody.Error)
	}

	resultBytes, err := json.Marshal(uploadBody.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result toolResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content[0].Text)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty tool content")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"slug": "hello-mcp"`) {
		t.Fatalf("expected custom slug in response, got: %s", text)
	}
	if !strings.Contains(text, "http://dash.example.com/d/hello-mcp") {
		t.Fatalf("expected absolute URL, got: %s", text)
	}

	meta, err := store.GetMeta("hello-mcp")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.Title != "Hello MCP" {
		t.Fatalf("title = %q", meta.Title)
	}
	if len(meta.Tags) != 2 {
		t.Fatalf("tags = %#v", meta.Tags)
	}

	thumbPath := filepath.Join(storeDashboardDir(t, store, "hello-mcp"), "thumb.png")
	info, err := os.Stat(thumbPath)
	if err != nil {
		t.Fatalf("thumb missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("thumb empty")
	}
}

func storeDashboardDir(t *testing.T, store *storage.Store, slug string) string {
	t.Helper()
	path, err := store.HTMLPath(slug)
	if err != nil {
		t.Fatalf("HTMLPath: %v", err)
	}
	return filepath.Dir(path)
}

func TestMCPTokenAuth(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.MCPToken = "secret-token"
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	req2.Header.Set("Authorization", "Bearer secret-token")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
}

func TestPlaceholderThumbPNG(t *testing.T) {
	data := placeholderThumbPNG()
	if len(data) < 100 {
		t.Fatalf("png too small: %d", len(data))
	}
	if data[0] != 0x89 || string(data[1:4]) != "PNG" {
		t.Fatal("not a PNG")
	}
}
