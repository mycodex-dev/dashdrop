package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/mycodex-dev/dashdrop/internal/config"
	"github.com/mycodex-dev/dashdrop/internal/handlers"
	"github.com/mycodex-dev/dashdrop/internal/middleware"
	"github.com/mycodex-dev/dashdrop/internal/storage"
)

//go:embed web/*
var webFS embed.FS

func main() {
	cfg := config.Load()

	store, err := storage.New(cfg.DataDir, cfg.PublicPathPrefix)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}

	static, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}

	h := handlers.New(store, cfg, static)
	uploadLimiter := middleware.NewRateLimiter(cfg.MaxUploadsPerMin, time.Minute)
	withUploadGuards := func(next http.Handler) http.Handler {
		return middleware.UploadRateLimit(uploadLimiter)(
			middleware.MaxBytes(cfg.MaxUploadRequestBytes())(next),
		)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/config", h.HandleConfig)
	mux.HandleFunc("GET /api/settings", h.HandleGetSettings)
	mux.HandleFunc("PUT /api/settings", h.HandleUpdateSettings)
	mux.HandleFunc("POST /api/settings/logo", h.HandleUploadLogo)
	mux.HandleFunc("DELETE /api/settings/logo", h.HandleDeleteLogo)
	mux.HandleFunc("GET /api/settings/logo", h.HandleServeLogo)
	mux.Handle("POST /api/upload", withUploadGuards(http.HandlerFunc(h.HandleUpload)))
	mux.HandleFunc("GET /api/dashboards", h.HandleList)
	mux.HandleFunc("GET /api/tags", h.HandleTags)
	mux.HandleFunc("GET /api/slugs/{slug}", h.HandleSlugCheck)
	mux.Handle("PUT /api/dashboards/{slug}", withUploadGuards(http.HandlerFunc(h.HandleReplace)))
	mux.HandleFunc("PATCH /api/dashboards/{slug}", h.HandleUpdateMeta)
	mux.HandleFunc("DELETE /api/dashboards/{slug}", h.HandleDelete)
	mux.HandleFunc("GET /api/dashboards/{slug}/download", h.HandleDownload)
	mux.HandleFunc("GET /api/dashboards/{slug}/thumb.png", h.HandleThumb)
	mux.HandleFunc(cfg.ServePattern(), h.HandleServe)
	mux.HandleFunc("GET /library", h.ServePage("library.html"))
	mux.HandleFunc("GET /settings", h.ServePage("settings.html"))
	mux.HandleFunc("GET /css/{file}", h.ServeStatic)
	mux.HandleFunc("GET /js/{file}", h.ServeStatic)
	mux.HandleFunc("GET /", h.ServePage("index.html"))

	addr := ":" + cfg.Port
	log.Printf("dashdrop listening on %s (data: %s, public: %s/{slug})", addr, cfg.DataDir, cfg.PublicPathPrefix)
	if err := http.ListenAndServe(addr, middleware.Logger(mux)); err != nil {
		log.Fatal(err)
	}
}
