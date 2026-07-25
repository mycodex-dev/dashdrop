package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port             string
	DataDir          string
	MaxUploadBytes   int64
	MaxThumbBytes    int64
	BaseURL          string
	MaxUploadsPerMin int
}

// MaxUploadRequestBytes is the max multipart body size (HTML + thumb + overhead).
func (c Config) MaxUploadRequestBytes() int64 {
	const multipartOverhead = 256 << 10 // 256 KiB for boundaries / headers
	return c.MaxUploadBytes + c.MaxThumbBytes + multipartOverhead
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
	}
}
