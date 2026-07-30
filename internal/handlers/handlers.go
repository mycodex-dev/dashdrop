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
	Slug      string     `json:"slug"`
	URL       string     `json:"url"`
	Title     string     `json:"title"`
	Tags      []string   `json:"tags,omitempty"`
	Archived  bool       `json:"archived,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// robotsTagNoIndex asks search engines and well-behaved crawlers not to index
// or archive the response. Uploaded HTML is served as-is, so headers (not meta
// tags) are the reliable signal.
const robotsTagNoIndex = "noindex, nofollow, noarchive, nosnippet"

// aiCrawlerUserAgents are known AI / training crawlers that honor robots.txt.
var aiCrawlerUserAgents = []string{
	"GPTBot",
	"Google-Extended",
	"ClaudeBot",
	"anthropic-ai",
	"Bytespider",
	"CCBot",
	"FacebookBot",
	"meta-externalagent",
	"Applebot-Extended",
	"PerplexityBot",
	"Diffbot",
}

func setNoIndexHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", robotsTagNoIndex)
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
		Slug:      entry.Slug,
		URL:       h.absoluteURL(r, entry.URL),
		Title:     entry.Title,
		Tags:      entry.Tags,
		Archived:  entry.Archived,
		ExpiresAt: entry.ExpiresAt,
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
		Slug:      entry.Slug,
		URL:       h.absoluteURL(r, entry.URL),
		Title:     entry.Title,
		Tags:      entry.Tags,
		Archived:  entry.Archived,
		ExpiresAt: entry.ExpiresAt,
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
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	Tags      *[]string `json:"tags"`
	Archived  *bool     `json:"archived"`
	ExpiresAt *string   `json:"expires_at"`
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

	var expiresAt *time.Time
	updateExpires := req.ExpiresAt != nil
	if updateExpires {
		parsed, err := storage.ParseExpiresAt(*req.ExpiresAt)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid expiration date (use YYYY-MM-DD or RFC3339)")
			return
		}
		expiresAt = parsed
	}

	archiveOnly := req.Archived != nil && req.Title == "" && req.Slug == "" && !updateTags && !updateExpires

	var entry storage.DashboardEntry
	var err error
	if archiveOnly {
		entry, err = h.store.SetArchived(currentSlug, *req.Archived)
	} else {
		entry, err = h.store.UpdateMeta(currentSlug, req.Slug, req.Title, tags, updateTags, expiresAt, updateExpires)
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
		case errors.Is(err, storage.ErrInvalidExpires):
			h.writeError(w, http.StatusBadRequest, "invalid expiration date")
		default:
			h.writeError(w, http.StatusInternalServerError, "failed to update dashboard")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, uploadResponse{
		Slug:      entry.Slug,
		URL:       h.absoluteURL(r, entry.URL),
		Title:     entry.Title,
		Tags:      entry.Tags,
		Archived:  entry.Archived,
		ExpiresAt: entry.ExpiresAt,
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

	setNoIndexHeaders(w)
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

	setNoIndexHeaders(w)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeFile(w, r, thumbPath)
}

func (h *Handler) HandleServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	htmlPath, err := h.store.AccessibleHTMLPath(slug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrArchived) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	setNoIndexHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, htmlPath)
}

// HandleRobots serves robots.txt that blocks crawlers from published dashboards
// and known AI training bots from those paths.
func (h *Handler) HandleRobots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	prefix := h.cfg.PublicPathPrefix
	var b strings.Builder
	b.WriteString("# Dashdrop: published dashboards are not for search/AI indexing\n")
	b.WriteString("User-agent: *\n")
	b.WriteString("Disallow: ")
	b.WriteString(prefix)
	b.WriteString("/\n")
	b.WriteString("Disallow: /library\n")
	b.WriteString("Disallow: /api/\n")
	b.WriteByte('\n')

	for _, ua := range aiCrawlerUserAgents {
		b.WriteString("User-agent: ")
		b.WriteString(ua)
		b.WriteByte('\n')
		b.WriteString("Disallow: ")
		b.WriteString(prefix)
		b.WriteString("/\n")
		b.WriteByte('\n')
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(b.String()))
}

func (h *Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"max_upload_bytes":   h.cfg.MaxUploadBytes,
		"public_path_prefix": h.cfg.PublicPathPrefix,
	})
}

type settingsResponse struct {
	AppName      string `json:"app_name"`
	PrimaryColor string `json:"primary_color"`
	HasLogo      bool   `json:"has_logo"`
	LogoURL      string `json:"logo_url,omitempty"`
}

func (h *Handler) settingsPayload(r *http.Request, settings storage.InstanceSettings) settingsResponse {
	hasLogo := settings.LogoFilename != ""
	if hasLogo {
		if _, _, err := h.store.LogoPath(); err != nil {
			hasLogo = false
		}
	}
	resp := settingsResponse{
		AppName:      settings.AppName,
		PrimaryColor: settings.PrimaryColor,
		HasLogo:      hasLogo,
	}
	if resp.HasLogo {
		resp.LogoURL = h.absoluteURL(r, "/api/settings/logo")
	}
	return resp
}

func (h *Handler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := h.store.GetSettings()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	h.writeJSON(w, http.StatusOK, h.settingsPayload(r, settings))
}

func (h *Handler) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		AppName      string `json:"app_name"`
		PrimaryColor string `json:"primary_color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	settings, err := h.store.SaveSettings(body.AppName, body.PrimaryColor)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidAppName) {
			h.writeError(w, http.StatusBadRequest, "app name must be 1–64 characters")
			return
		}
		if errors.Is(err, storage.ErrInvalidPrimaryColor) {
			h.writeError(w, http.StatusBadRequest, "primary color must be a hex value like #0f766e")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	h.writeJSON(w, http.StatusOK, h.settingsPayload(r, settings))
}

func (h *Handler) HandleUploadLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxLogoRequest = storage.MaxLogoBytes + (256 << 10)
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoRequest)
	if err := r.ParseMultipartForm(maxLogoRequest); err != nil {
		h.writeError(w, http.StatusBadRequest, "request too large or invalid")
		return
	}
	defer cleanupMultipart(r)

	file, header, err := r.FormFile("logo")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "logo file is required")
		return
	}
	defer file.Close()

	settings, err := h.store.SaveLogo(header.Filename, file, header.Size)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidLogo) {
			h.writeError(w, http.StatusBadRequest, "logo must be png, jpg, webp, gif, or svg")
			return
		}
		if errors.Is(err, storage.ErrTooLarge) {
			h.writeError(w, http.StatusBadRequest, "logo exceeds maximum size (512 KB)")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "failed to save logo")
		return
	}
	h.writeJSON(w, http.StatusOK, h.settingsPayload(r, settings))
}

func (h *Handler) HandleDeleteLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := h.store.DeleteLogo()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to remove logo")
		return
	}
	h.writeJSON(w, http.StatusOK, h.settingsPayload(r, settings))
}

func (h *Handler) HandleServeLogo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, contentType, err := h.store.LogoPath()
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load logo", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeFile(w, r, path)
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
