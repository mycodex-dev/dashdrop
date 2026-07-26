package config

import "testing"

func TestNormalizePublicPathPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/d"},
		{"/d", "/d"},
		{"d", "/d"},
		{"/d/", "/d"},
		{" /View/ ", "/view"},
		{"p", "/p"},
		{"/drop", "/drop"},
		{"/dash/boards", "/dash/boards"},
		{"/", "/d"},
		{"//", "/d"},
		{"/api", "/d"},
		{"/library", "/d"},
		{"/settings", "/d"},
		{"/mcp", "/d"},
		{"/api/foo", "/d"},
		{"bad path", "/d"},
		{"/-bad", "/d"},
		{"/ok-path", "/ok-path"},
	}
	for _, tt := range tests {
		got := NormalizePublicPathPrefix(tt.in)
		if got != tt.want {
			t.Errorf("NormalizePublicPathPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestServePatternAndPublicURL(t *testing.T) {
	cfg := Config{PublicPathPrefix: "/view"}
	if got := cfg.ServePattern(); got != "GET /view/{slug}" {
		t.Errorf("ServePattern() = %q", got)
	}
	if got := cfg.PublicURL("abc123"); got != "/view/abc123" {
		t.Errorf("PublicURL() = %q", got)
	}
}
