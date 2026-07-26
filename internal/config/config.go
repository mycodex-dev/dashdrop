package config

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

const DefaultPublicPathPrefix = "/drop"

var publicPathPrefixRe = regexp.MustCompile(`(?i)^/[a-z0-9]([a-z0-9_-]*[a-z0-9])?(/[a-z0-9]([a-z0-9_-]*[a-z0-9])?)*$`)

// Reserved top-level routes that cannot be used as the public path prefix.
var reservedPublicPrefixes = map[string]bool{
	"/api": true, "/library": true, "/settings": true,
	"/css": true, "/js": true, "/branding": true,
	"/admin": true, "/health": true, "/static": true, "/assets": true,
}

type Config struct {
	Port             string
	DataDir          string
	MaxUploadBytes   int64
	MaxThumbBytes    int64
	BaseURL          string
	MaxUploadsPerMin int
	PublicPathPrefix string
}

// MaxUploadRequestBytes is the max multipart body size (HTML + thumb + overhead).
func (c Config) MaxUploadRequestBytes() int64 {
	const multipartOverhead = 256 << 10 // 256 KiB for boundaries / headers
	return c.MaxUploadBytes + c.MaxThumbBytes + multipartOverhead
}

// PublicURL returns the relative public path for a dashboard slug.
func (c Config) PublicURL(slug string) string {
	return c.PublicPathPrefix + "/" + slug
}

// ServePattern returns the Go ServeMux pattern for serving published dashboards.
func (c Config) ServePattern() string {
	return "GET " + c.PublicPathPrefix + "/{slug}"
}

func Load() Config {
	maxBytes := int64(5242880)
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBytes = n
		}
	}

	maxThumb := int64(1 << 20) // 1 MiB — enough for 1280x800 PNG previews
	if v := os.Getenv("MAX_THUMB_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxThumb = n
		}
	}

	maxUploads := 10
	if v := os.Getenv("MAX_UPLOADS_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxUploads = n
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	return Config{
		Port:             port,
		DataDir:          dataDir,
		MaxUploadBytes:   maxBytes,
		MaxThumbBytes:    maxThumb,
		BaseURL:          os.Getenv("BASE_URL"),
		MaxUploadsPerMin: maxUploads,
		PublicPathPrefix: NormalizePublicPathPrefix(os.Getenv("PUBLIC_PATH_PREFIX")),
	}
}

// NormalizePublicPathPrefix cleans and validates a public path prefix.
// Invalid or empty values fall back to DefaultPublicPathPrefix ("/drop").
func NormalizePublicPathPrefix(raw string) string {
	prefix := strings.TrimSpace(raw)
	if prefix == "" {
		return DefaultPublicPathPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return DefaultPublicPathPrefix
	}
	prefix = strings.ToLower(prefix)
	if !publicPathPrefixRe.MatchString(prefix) {
		return DefaultPublicPathPrefix
	}
	if reservedPublicPrefixes[prefix] {
		return DefaultPublicPathPrefix
	}
	// Also reject reserved first segments (e.g. /api/foo).
	first := prefix
	if i := strings.Index(prefix[1:], "/"); i >= 0 {
		first = prefix[:i+1]
	}
	if reservedPublicPrefixes[first] {
		return DefaultPublicPathPrefix
	}
	return prefix
}
