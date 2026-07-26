package config

import "testing"

func TestNormalizePublicPathPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/drop"},
		{"/drop", "/drop"},
		{"drop", "/drop"},
		{"/drop/", "/drop"},
		{" /View/ ", "/view"},
		{"p", "/p"},
		{"/d", "/d"},
		{"/dash/boards", "/dash/boards"},
		{"/", "/drop"},
		{"//", "/drop"},
		{"/api", "/drop"},
		{"/library", "/drop"},
		{"/settings", "/drop"},
		{"/api/foo", "/drop"},
		{"bad path", "/drop"},
		{"/-bad", "/drop"},
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
