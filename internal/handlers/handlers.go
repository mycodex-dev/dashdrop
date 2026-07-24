package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mycodex-dev/dashdrop/internal/config"
	"github.com/mycodex-dev/dashdrop/internal/storage"
)

type Handler struct {
	store  *storage.Store
	cfg    config.Config
	static fs.FS
}

func New(store *storage.Store, cfg config.Config, static fs.FS) *Handler {
	return &Handler{store: store, cfg: cfg, static: static}
}

type uploadResponse struct {
	Slug  string `json:"slug"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, errorResponse{Error: msg})
}

func (h *Handler) absoluteURL(r *http.Request, path string) string {
	if h.cfg.BaseURL != "" {
		return strings.TrimRight(h.cfg.BaseURL, "/") + path
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	return scheme + "://" + r.Host + path
}

func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(h.cfg.MaxUploadBytes + 1024*1024); err != nil {
		h.writeError(w, http.StatusBadRequest, "request too large or invalid")
		return
	}

	htmlFile, htmlHeader, err := r.FormFile("html")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "html file is required")
		return
	}
	defer htmlFile.Close()

	thumbFile, _, err := r.FormFile("thumb")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "thumbnail is required")
		return
	}
	defer thumbFile.Close()

	filename := htmlHeader.Filename
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".html" && ext != ".htm" {
		h.writeError(w, http.StatusBadRequest, "only .html files are allowed")
		return
	}

	if htmlHeader.Size > h.cfg.MaxUploadBytes {
		h.writeError(w, http.StatusBadRequest, "file exceeds maximum size")
		return
	}

	contentType := htmlHeader.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if mediaType != "text/html" && mediaType != "application/octet-stream" {
			h.writeError(w, http.StatusBadRequest, "invalid content type")
			return
		}
	}

	entry, err := h.store.SaveUpload(filename, htmlFile, htmlHeader.Size, thumbFile)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to save upload")
		return
	}

	h.writeJSON(w, http.StatusCreated, uploadResponse{
		Slug:  entry.Slug,
		URL:   h.absoluteURL(r, entry.URL),
		Title: entry.Title,
	})
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := h.store.List()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to list dashboards")
		return
	}
	if entries == nil {
		entries = []storage.DashboardEntry{}
	}
	h.writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/api/dashboards/")
	if slug == "" || strings.Contains(slug, "/") {
		h.writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	err := h.store.Delete(slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		if errors.Is(err, storage.ErrInvalidSlug) {
			h.writeError(w, http.StatusBadRequest, "invalid slug")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to delete dashboard")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleThumb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/dashboards/")
	path = strings.TrimSuffix(path, "/thumb.png")
	if path == "" || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}

	thumbPath, err := h.store.ThumbPath(path)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, thumbPath)
}

func (h *Handler) HandleServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/d/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	htmlPath, err := h.store.HTMLPath(slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, htmlPath)
}

func (h *Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]int64{
		"max_upload_bytes": h.cfg.MaxUploadBytes,
	})
}

func (h *Handler) ServeStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	file, err := h.static.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if strings.HasPrefix(path, "css/") || strings.HasPrefix(path, "js/") {
		w.Header().Set("Cache-Control", "no-cache")
	}

	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, stat.Name(), time.Time{}, rs)
		return
	}
	io.Copy(w, file)
}

func (h *Handler) ServePage(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, err := h.static.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.Copy(w, file)
	}
}
