package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port             string
	DataDir          string
	MaxUploadBytes   int64
	BaseURL          string
	MaxUploadsPerMin int
}

func Load() Config {
	maxBytes := int64(5242880)
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBytes = n
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
		BaseURL:          os.Getenv("BASE_URL"),
		MaxUploadsPerMin: maxUploads,
	}
}
