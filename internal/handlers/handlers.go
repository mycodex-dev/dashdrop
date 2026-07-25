package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
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
	Slug     string   `json:"slug"`
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags,omitempty"`
	Archived bool     `json:"archived,omitempty"`
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

	filename, htmlFile, htmlHeader, thumbFile, ok := h.parseUploadForm(w, r)
	if !ok {
		return
	}
	defer htmlFile.Close()
	defer thumbFile.Close()
	defer cleanupMultipart(r)

	entry, err := h.store.SaveUpload(filename, htmlFile, htmlHeader.Size, thumbFile, h.cfg.MaxUploadBytes, h.cfg.MaxThumbBytes)
	if err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			h.writeError(w, http.StatusBadRequest, "file exceeds maximum size")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to save upload")
		return
	}

	h.writeJSON(w, http.StatusCreated, uploadResponse{
		Slug:     entry.Slug,
		URL:      h.absoluteURL(r, entry.URL),
		Title:    entry.Title,
		Tags:     entry.Tags,
		Archived: entry.Archived,
	})
}

func (h *Handler) HandleReplace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		h.writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	filename, htmlFile, htmlHeader, thumbFile, ok := h.parseUploadForm(w, r)
	if !ok {
		return
	}
	defer htmlFile.Close()
	defer thumbFile.Close()
	defer cleanupMultipart(r)

	entry, err := h.store.ReplaceUpload(slug, filename, htmlFile, htmlHeader.Size, thumbFile, h.cfg.MaxUploadBytes, h.cfg.MaxThumbBytes)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "dashboard not found")
			return
		}
		if errors.Is(err, storage.ErrInvalidSlug) {
			h.writeError(w, http.StatusBadRequest, "invalid slug")
			return
		}
		if errors.Is(err, storage.ErrTooLarge) {
			h.writeError(w, http.StatusBadRequest, "file exceeds maximum size")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to replace upload")
		return
	}

	h.writeJSON(w, http.StatusOK, uploadResponse{
		Slug:     entry.Slug,
		URL:      h.absoluteURL(r, entry.URL),
		Title:    entry.Title,
		Tags:     entry.Tags,
		Archived: entry.Archived,
	})
}

func cleanupMultipart(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

func (h *Handler) parseUploadForm(w http.ResponseWriter, r *http.Request) (string, multipart.File, *multipart.FileHeader, multipart.File, bool) {
	maxMemory := h.cfg.MaxUploadBytes + h.cfg.MaxThumbBytes
	if maxMemory > 32<<20 {
		maxMemory = 32 << 20
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "request too large")
			return "", nil, nil, nil, false
		}
		h.writeError(w, http.StatusBadRequest, "request too large or invalid")
		return "", nil, nil, nil, false
	}

	htmlFile, htmlHeader, err := r.FormFile("html")
	if err != nil {
		cleanupMultipart(r)
		h.writeError(w, http.StatusBadRequest, "html file is required")
		return "", nil, nil, nil, false
	}

	thumbFile, thumbHeader, err := r.FormFile("thumb")
	if err != nil {
		htmlFile.Close()
		cleanupMultipart(r)
		h.writeError(w, http.StatusBadRequest, "thumbnail is required")
		return "", nil, nil, nil, false
	}

	filename := htmlHeader.Filename
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".html" && ext != ".htm" {
		htmlFile.Close()
		thumbFile.Close()
		cleanupMultipart(r)
		h.writeError(w, http.StatusBadRequest, "only .html files are allowed")
		return "", nil, nil, nil, false
	}

	if htmlHeader.Size < 0 || htmlHeader.Size > h.cfg.MaxUploadBytes {
		htmlFile.Close()
		thumbFile.Close()
		cleanupMultipart(r)
		h.writeError(w, http.StatusBadRequest, "file exceeds maximum size")
		return "", nil, nil, nil, false
	}

	if thumbHeader.Size < 0 || thumbHeader.Size > h.cfg.MaxThumbBytes {
		htmlFile.Close()
		thumbFile.Close()
		cleanupMultipart(r)
		h.writeError(w, http.StatusBadRequest, "thumbnail exceeds maximum size")
		return "", nil, nil, nil, false
	}

	contentType := htmlHeader.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if mediaType != "text/html" && mediaType != "application/octet-stream" {
			htmlFile.Close()
			thumbFile.Close()
			cleanupMultipart(r)
			h.writeError(w, http.StatusBadRequest, "invalid content type")
			return "", nil, nil, nil, false
		}
	}

	return filename, htmlFile, htmlHeader, thumbFile, true
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	archivedOnly := r.URL.Query().Get("archived") == "1" ||
		strings.EqualFold(r.URL.Query().Get("archived"), "true")

	var entries []storage.DashboardEntry
	var err error
	switch {
	case archivedOnly:
		entries, err = h.store.ListArchived()
	case tag != "":
		entries, err = h.store.ListByTag(tag)
	default:
		entries, err = h.store.List()
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to list dashboards")
		return
	}
	if entries == nil {
		entries = []storage.DashboardEntry{}
	}
	h.writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) HandleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tags, err := h.store.ListTags()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	if tags == nil {
		tags = []string{}
	}
	h.writeJSON(w, http.StatusOK, tags)
}

func (h *Handler) HandleSlugCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := storage.NormalizeSlug(r.PathValue("slug"))
	except := storage.NormalizeSlug(r.URL.Query().Get("except"))

	if !storage.ValidSlug(slug) {
		h.writeJSON(w, http.StatusOK, map[string]any{
			"slug":      slug,
			"available": false,
			"valid":     false,
			"error":     "use 2–48 characters: lowercase letters, numbers, and hyphens",
		})
		return
	}

	available, err := h.store.SlugAvailable(slug, except)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidSlug) {
			h.writeJSON(w, http.StatusOK, map[string]any{
				"slug":      slug,
				"available": false,
				"valid":     false,
				"error":     "invalid slug",
			})
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to check slug")
		return
	}

	resp := map[string]any{
		"slug":      slug,
		"available": available,
		"valid":     true,
	}
	if !available {
		resp["error"] = "slug is already taken"
	}
	h.writeJSON(w, http.StatusOK, resp)
}

type updateMetaRequest struct {
	Title    string    `json:"title"`
	Slug     string    `json:"slug"`
	Tags     *[]string `json:"tags"`
	Archived *bool     `json:"archived"`
}

func (h *Handler) HandleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSlug := r.PathValue("slug")
	if currentSlug == "" {
		h.writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	var req updateMetaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var tags []string
	updateTags := req.Tags != nil
	if updateTags {
		tags = *req.Tags
	}

	archiveOnly := req.Archived != nil && req.Title == "" && req.Slug == "" && !updateTags

	var entry storage.DashboardEntry
	var err error
	if archiveOnly {
		entry, err = h.store.SetArchived(currentSlug, *req.Archived)
	} else {
		entry, err = h.store.UpdateMeta(currentSlug, req.Slug, req.Title, tags, updateTags)
		if err == nil && req.Archived != nil {
			entry, err = h.store.SetArchived(entry.Slug, *req.Archived)
		}
	}
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			h.writeError(w, http.StatusNotFound, "dashboard not found")
		case errors.Is(err, storage.ErrSlugTaken):
			h.writeError(w, http.StatusConflict, "slug is already taken")
		case errors.Is(err, storage.ErrInvalidSlug):
			h.writeError(w, http.StatusBadRequest, "invalid slug")
		case errors.Is(err, storage.ErrInvalidTitle):
			h.writeError(w, http.StatusBadRequest, "title is required")
		case errors.Is(err, storage.ErrInvalidTags):
			h.writeError(w, http.StatusBadRequest, "invalid tags (max 10, each up to 32 chars: letters, numbers, hyphens, underscores)")
		default:
			h.writeError(w, http.StatusInternalServerError, "failed to update dashboard")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, uploadResponse{
		Slug:     entry.Slug,
		URL:      h.absoluteURL(r, entry.URL),
		Title:    entry.Title,
		Tags:     entry.Tags,
		Archived: entry.Archived,
	})
}

func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
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

func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
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

	filename := slug + ".html"
	if meta, err := h.store.GetMeta(slug); err == nil {
		if meta.OriginalFilename != "" {
			filename = filepath.Base(meta.OriginalFilename)
		} else if meta.Title != "" {
			filename = meta.Title + ".html"
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeDownloadName(filename)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, htmlPath)
}

func sanitizeDownloadName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	if name == "" || name == "." || name == ".." {
		return "dashboard.html"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".html") && !strings.HasSuffix(strings.ToLower(name), ".htm") {
		name += ".html"
	}
	return name
}

func (h *Handler) HandleThumb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	thumbPath, err := h.store.ThumbPath(slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
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
