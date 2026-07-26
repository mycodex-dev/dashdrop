package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mycodex-dev/dashdrop/internal/storage"
)

func toolDefinitions() []toolDef {
	return []toolDef{
		{
			Name:        "upload_dashboard",
			Description: "Upload a single-file HTML dashboard and publish it to a live URL. The server generates a thumbnail automatically.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"html": map[string]any{
						"type":        "string",
						"description": "Full HTML document to publish",
					},
					"filename": map[string]any{
						"type":        "string",
						"description": "Original filename ending in .html (used as default title). Defaults to dashboard.html",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Optional display title (overrides filename-derived title)",
					},
					"slug": map[string]any{
						"type":        "string",
						"description": "Optional custom URL slug (2–48 chars: lowercase letters, numbers, hyphens)",
					},
					"tags": map[string]any{
						"type":        "array",
						"description": "Optional tags (max 10)",
						"items":       map[string]any{"type": "string"},
					},
					"expires_at": map[string]any{
						"type":        "string",
						"description": "Optional expiration as YYYY-MM-DD or RFC3339",
					},
				},
				"required": []string{"html"},
			},
		},
		{
			Name:        "replace_dashboard",
			Description: "Replace an existing dashboard's HTML while preserving tags/archive/expiry unless updated afterward. The server regenerates the thumbnail.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{
						"type":        "string",
						"description": "Dashboard slug to replace",
					},
					"html": map[string]any{
						"type":        "string",
						"description": "Full HTML document to publish",
					},
					"filename": map[string]any{
						"type":        "string",
						"description": "Original filename ending in .html",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Optional new display title",
					},
					"tags": map[string]any{
						"type":        "array",
						"description": "Optional replacement tags",
						"items":       map[string]any{"type": "string"},
					},
					"expires_at": map[string]any{
						"type":        "string",
						"description": "Optional expiration as YYYY-MM-DD or RFC3339; empty string clears",
					},
				},
				"required": []string{"slug", "html"},
			},
		},
		{
			Name:        "list_dashboards",
			Description: "List published dashboards. By default returns active (non-archived) dashboards.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tag": map[string]any{
						"type":        "string",
						"description": "Optional tag filter",
					},
					"archived": map[string]any{
						"type":        "boolean",
						"description": "If true, list archived dashboards only",
					},
				},
			},
		},
		{
			Name:        "get_dashboard",
			Description: "Get metadata for a dashboard by slug. Optionally include the HTML source.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{
						"type":        "string",
						"description": "Dashboard slug",
					},
					"include_html": map[string]any{
						"type":        "boolean",
						"description": "If true, include the HTML source in the response",
					},
				},
				"required": []string{"slug"},
			},
		},
		{
			Name:        "update_dashboard",
			Description: "Update dashboard metadata: title, slug, tags, archived, and/or expires_at.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{
						"type":        "string",
						"description": "Current dashboard slug",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "New title (required unless only changing archived)",
					},
					"new_slug": map[string]any{
						"type":        "string",
						"description": "Optional new URL slug",
					},
					"tags": map[string]any{
						"type":        "array",
						"description": "Replacement tags",
						"items":       map[string]any{"type": "string"},
					},
					"archived": map[string]any{
						"type":        "boolean",
						"description": "Archive or unarchive the dashboard",
					},
					"expires_at": map[string]any{
						"type":        "string",
						"description": "Expiration as YYYY-MM-DD or RFC3339; empty string clears",
					},
				},
				"required": []string{"slug"},
			},
		},
		{
			Name:        "delete_dashboard",
			Description: "Permanently delete a dashboard by slug.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{
						"type":        "string",
						"description": "Dashboard slug to delete",
					},
				},
				"required": []string{"slug"},
			},
		},
	}
}

func (s *Server) callTool(r *http.Request, params callToolParams) toolResult {
	args := params.Arguments
	if args == nil {
		args = map[string]any{}
	}

	switch params.Name {
	case "upload_dashboard":
		return s.uploadDashboard(r, args)
	case "replace_dashboard":
		return s.replaceDashboard(r, args)
	case "list_dashboards":
		return s.listDashboards(args)
	case "get_dashboard":
		return s.getDashboard(r, args)
	case "update_dashboard":
		return s.updateDashboard(r, args)
	case "delete_dashboard":
		return s.deleteDashboard(args)
	default:
		return errorResult("unknown tool: " + params.Name)
	}
}

func (s *Server) uploadDashboard(r *http.Request, args map[string]any) toolResult {
	html, err := requireString(args, "html")
	if err != nil {
		return errorResult(err.Error())
	}
	if int64(len(html)) > s.cfg.MaxUploadBytes {
		return errorResult("html exceeds maximum size")
	}

	filename := optionalString(args, "filename")
	if filename == "" {
		filename = "dashboard.html"
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != ".html" && ext != ".htm" {
		return errorResult("filename must end with .html or .htm")
	}

	entry, err := s.store.SaveUpload(filename, strings.NewReader(html), int64(len(html)), nil, s.cfg.MaxUploadBytes, s.cfg.MaxThumbBytes)
	if err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return errorResult("file exceeds maximum size")
		}
		return errorResult("failed to save upload: " + err.Error())
	}

	entry, err = s.applyOptionalMeta(entry.Slug, args, true)
	if err != nil {
		return errorResult(err.Error())
	}

	return textResult(formatToolJSON(s.dashboardPayload(r, entry)))
}

func (s *Server) replaceDashboard(r *http.Request, args map[string]any) toolResult {
	slug, err := requireString(args, "slug")
	if err != nil {
		return errorResult(err.Error())
	}
	html, err := requireString(args, "html")
	if err != nil {
		return errorResult(err.Error())
	}
	if int64(len(html)) > s.cfg.MaxUploadBytes {
		return errorResult("html exceeds maximum size")
	}

	filename := optionalString(args, "filename")
	if filename == "" {
		filename = slug + ".html"
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != ".html" && ext != ".htm" {
		return errorResult("filename must end with .html or .htm")
	}

	entry, err := s.store.ReplaceUpload(storage.NormalizeSlug(slug), filename, strings.NewReader(html), int64(len(html)), nil, s.cfg.MaxUploadBytes, s.cfg.MaxThumbBytes)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			return errorResult("dashboard not found")
		case errors.Is(err, storage.ErrInvalidSlug):
			return errorResult("invalid slug")
		case errors.Is(err, storage.ErrTooLarge):
			return errorResult("file exceeds maximum size")
		default:
			return errorResult("failed to replace upload: " + err.Error())
		}
	}

	entry, err = s.applyOptionalMeta(entry.Slug, args, false)
	if err != nil {
		return errorResult(err.Error())
	}

	return textResult(formatToolJSON(s.dashboardPayload(r, entry)))
}

func (s *Server) listDashboards(args map[string]any) toolResult {
	tag := optionalString(args, "tag")
	archived, _ := args["archived"].(bool)

	var (
		entries []storage.DashboardEntry
		err     error
	)
	switch {
	case archived:
		entries, err = s.store.ListArchived()
	case tag != "":
		entries, err = s.store.ListByTag(tag)
	default:
		entries, err = s.store.List()
	}
	if err != nil {
		return errorResult("failed to list dashboards: " + err.Error())
	}
	if entries == nil {
		entries = []storage.DashboardEntry{}
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, s.dashboardPayload(nil, e))
	}
	return textResult(formatToolJSON(out))
}

func (s *Server) getDashboard(r *http.Request, args map[string]any) toolResult {
	slug, err := requireString(args, "slug")
	if err != nil {
		return errorResult(err.Error())
	}
	includeHTML, _ := args["include_html"].(bool)

	meta, err := s.store.GetMeta(storage.NormalizeSlug(slug))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return errorResult("dashboard not found")
		}
		if errors.Is(err, storage.ErrInvalidSlug) {
			return errorResult("invalid slug")
		}
		return errorResult("failed to load dashboard: " + err.Error())
	}

	entry := storage.DashboardEntry{
		Slug:      meta.Slug,
		Title:     meta.Title,
		Tags:      meta.Tags,
		Archived:  meta.Archived,
		ExpiresAt: meta.ExpiresAt,
		CreatedAt: meta.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
		ThumbURL:  "/api/dashboards/" + meta.Slug + "/thumb.png",
		URL:       s.cfg.PublicURL(meta.Slug),
	}

	payload := s.dashboardPayload(r, entry)
	payload["original_filename"] = meta.OriginalFilename
	payload["size_bytes"] = meta.SizeBytes

	if includeHTML {
		path, err := s.store.HTMLPath(meta.Slug)
		if err != nil {
			return errorResult("failed to read html: " + err.Error())
		}
		data, err := readFileLimited(path, s.cfg.MaxUploadBytes)
		if err != nil {
			return errorResult("failed to read html: " + err.Error())
		}
		payload["html"] = string(data)
	}

	return textResult(formatToolJSON(payload))
}

func (s *Server) updateDashboard(r *http.Request, args map[string]any) toolResult {
	slug, err := requireString(args, "slug")
	if err != nil {
		return errorResult(err.Error())
	}
	slug = storage.NormalizeSlug(slug)

	title := optionalString(args, "title")
	newSlug := optionalString(args, "new_slug")
	_, hasTags := args["tags"]
	tags := stringSlice(args["tags"])
	_, hasExpires := args["expires_at"]
	var expiresAt *time.Time
	if hasExpires {
		expiresAt, err = storage.ParseExpiresAt(optionalString(args, "expires_at"))
		if err != nil {
			return errorResult("invalid expiration date (use YYYY-MM-DD or RFC3339)")
		}
	}
	archived, hasArchived := args["archived"].(bool)

	archiveOnly := hasArchived && title == "" && newSlug == "" && !hasTags && !hasExpires
	var entry storage.DashboardEntry
	if archiveOnly {
		entry, err = s.store.SetArchived(slug, archived)
	} else {
		if title == "" {
			meta, metaErr := s.store.GetMeta(slug)
			if metaErr != nil {
				if errors.Is(metaErr, storage.ErrNotFound) {
					return errorResult("dashboard not found")
				}
				return errorResult("failed to load dashboard: " + metaErr.Error())
			}
			title = meta.Title
		}
		entry, err = s.store.UpdateMeta(slug, newSlug, title, tags, hasTags, expiresAt, hasExpires)
		if err == nil && hasArchived {
			entry, err = s.store.SetArchived(entry.Slug, archived)
		}
	}
	if err != nil {
		return errorResult(mapStorageErr(err))
	}
	return textResult(formatToolJSON(s.dashboardPayload(r, entry)))
}

func (s *Server) deleteDashboard(args map[string]any) toolResult {
	slug, err := requireString(args, "slug")
	if err != nil {
		return errorResult(err.Error())
	}
	if err := s.store.Delete(storage.NormalizeSlug(slug)); err != nil {
		return errorResult(mapStorageErr(err))
	}
	return textResult(formatToolJSON(map[string]any{
		"deleted": true,
		"slug":    storage.NormalizeSlug(slug),
	}))
}

func (s *Server) applyOptionalMeta(slug string, args map[string]any, forUpload bool) (storage.DashboardEntry, error) {
	title := optionalString(args, "title")
	customSlug := optionalString(args, "slug")
	if !forUpload {
		customSlug = "" // replace uses existing slug; title/tags/expires only
	}
	_, hasTags := args["tags"]
	tags := stringSlice(args["tags"])
	_, hasExpires := args["expires_at"]

	needsUpdate := title != "" || customSlug != "" || hasTags || hasExpires
	if !needsUpdate {
		meta, err := s.store.GetMeta(slug)
		if err != nil {
			return storage.DashboardEntry{}, err
		}
		return storage.DashboardEntry{
			Slug:      meta.Slug,
			Title:     meta.Title,
			Tags:      meta.Tags,
			Archived:  meta.Archived,
			ExpiresAt: meta.ExpiresAt,
			CreatedAt: meta.CreatedAt,
			UpdatedAt: meta.UpdatedAt,
			ThumbURL:  "/api/dashboards/" + meta.Slug + "/thumb.png",
			URL:       s.cfg.PublicURL(meta.Slug),
		}, nil
	}

	if title == "" {
		meta, err := s.store.GetMeta(slug)
		if err != nil {
			return storage.DashboardEntry{}, err
		}
		title = meta.Title
	}

	var expiresAt *time.Time
	if hasExpires {
		parsed, err := storage.ParseExpiresAt(optionalString(args, "expires_at"))
		if err != nil {
			return storage.DashboardEntry{}, fmt.Errorf("invalid expiration date (use YYYY-MM-DD or RFC3339)")
		}
		expiresAt = parsed
	}

	entry, err := s.store.UpdateMeta(slug, customSlug, title, tags, hasTags, expiresAt, hasExpires)
	if err != nil {
		return storage.DashboardEntry{}, errors.New(mapStorageErr(err))
	}
	return entry, nil
}

func (s *Server) dashboardPayload(r *http.Request, entry storage.DashboardEntry) map[string]any {
	url := entry.URL
	if r != nil {
		url = s.absoluteURL(r, entry.URL)
	} else if s.cfg.BaseURL != "" {
		url = strings.TrimRight(s.cfg.BaseURL, "/") + entry.URL
	}
	out := map[string]any{
		"slug":      entry.Slug,
		"title":     entry.Title,
		"url":       url,
		"thumb_url": entry.ThumbURL,
		"archived":  entry.Archived,
	}
	if len(entry.Tags) > 0 {
		out["tags"] = entry.Tags
	}
	if entry.ExpiresAt != nil {
		out["expires_at"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if !entry.CreatedAt.IsZero() {
		out["created_at"] = entry.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !entry.UpdatedAt.IsZero() {
		out["updated_at"] = entry.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) absoluteURL(r *http.Request, path string) string {
	if s.cfg.BaseURL != "" {
		return strings.TrimRight(s.cfg.BaseURL, "/") + path
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

func requireString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return s, nil
}

func optionalString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func stringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func mapStorageErr(err error) string {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return "dashboard not found"
	case errors.Is(err, storage.ErrSlugTaken):
		return "slug is already taken"
	case errors.Is(err, storage.ErrInvalidSlug):
		return "invalid slug"
	case errors.Is(err, storage.ErrInvalidTitle):
		return "title is required"
	case errors.Is(err, storage.ErrInvalidTags):
		return "invalid tags (max 10, each up to 32 chars: letters, numbers, hyphens, underscores)"
	case errors.Is(err, storage.ErrInvalidExpires):
		return "invalid expiration date"
	default:
		return "operation failed: " + err.Error()
	}
}

func parseCallToolParams(raw json.RawMessage) (callToolParams, error) {
	var params callToolParams
	if len(raw) == 0 {
		return params, fmt.Errorf("params required")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("invalid params")
	}
	if params.Name == "" {
		return params, fmt.Errorf("tool name is required")
	}
	return params, nil
}
