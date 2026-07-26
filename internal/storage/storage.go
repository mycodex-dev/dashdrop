package storage

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const slugChars = "abcdefghijklmnopqrstuvwxyz0123456789"
const slugLength = 6
const minCustomSlugLen = 2
const maxCustomSlugLen = 48
const maxTagLen = 32
const maxTagsPerDashboard = 10

var ErrNotFound = errors.New("dashboard not found")
var ErrInvalidSlug = errors.New("invalid slug")
var ErrSlugTaken = errors.New("slug already taken")
var ErrInvalidTitle = errors.New("invalid title")
var ErrInvalidTags = errors.New("invalid tags")
var ErrInvalidExpires = errors.New("invalid expiration date")
var ErrArchived = errors.New("dashboard archived")
var ErrTooLarge = errors.New("file exceeds maximum size")

var reservedSlugs = map[string]bool{
	"api": true, "library": true, "css": true, "js": true, "d": true,
	"admin": true, "health": true, "static": true, "assets": true,
	"settings": true, "branding": true,
	"favicon.ico": true, "robots.txt": true,
}

type DashboardMeta struct {
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Tags             []string   `json:"tags,omitempty"`
	Archived         bool       `json:"archived,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	OriginalFilename string     `json:"original_filename"`
	SizeBytes        int64      `json:"size_bytes"`
}

type DashboardEntry struct {
	Slug      string     `json:"slug"`
	Title     string     `json:"title"`
	Tags      []string   `json:"tags,omitempty"`
	Archived  bool       `json:"archived,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ThumbURL  string     `json:"thumb_url"`
	URL       string     `json:"url"`
}

// MarshalJSON omits zero updated_at values (time.Time omitempty is unreliable).
func (e DashboardEntry) MarshalJSON() ([]byte, error) {
	type entryJSON struct {
		Slug      string     `json:"slug"`
		Title     string     `json:"title"`
		Tags      []string   `json:"tags,omitempty"`
		Archived  bool       `json:"archived,omitempty"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt *time.Time `json:"updated_at,omitempty"`
		ThumbURL  string     `json:"thumb_url"`
		URL       string     `json:"url"`
	}
	out := entryJSON{
		Slug:      e.Slug,
		Title:     e.Title,
		Tags:      e.Tags,
		Archived:  e.Archived,
		ExpiresAt: copyTimePtr(e.ExpiresAt),
		CreatedAt: e.CreatedAt,
		ThumbURL:  e.ThumbURL,
		URL:       e.URL,
	}
	if !e.UpdatedAt.IsZero() {
		t := e.UpdatedAt
		out.UpdatedAt = &t
	}
	return json.Marshal(out)
}

type Store struct {
	dataDir          string
	publicPathPrefix string
	mu               sync.Mutex
}

func New(dataDir, publicPathPrefix string) (*Store, error) {
	dashboardsDir := filepath.Join(dataDir, "dashboards")
	if err := os.MkdirAll(dashboardsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dashboards dir: %w", err)
	}
	brandingDir := filepath.Join(dataDir, "branding")
	if err := os.MkdirAll(brandingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create branding dir: %w", err)
	}

	prefix := strings.TrimSpace(publicPathPrefix)
	if prefix == "" {
		prefix = "/d"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		prefix = "/d"
	}

	s := &Store{dataDir: dataDir, publicPathPrefix: prefix}
	if _, err := os.Stat(s.manifestPath()); os.IsNotExist(err) {
		if err := s.writeManifest([]DashboardEntry{}); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) manifestPath() string {
	return filepath.Join(s.dataDir, "manifest.json")
}

func (s *Store) dashboardDir(slug string) string {
	return filepath.Join(s.dataDir, "dashboards", slug)
}

func ValidSlug(slug string) bool {
	n := len(slug)
	if n < minCustomSlugLen || n > maxCustomSlugLen {
		return false
	}
	if reservedSlugs[slug] {
		return false
	}
	if slug[0] == '-' || slug[n-1] == '-' {
		return false
	}
	for i := 0; i < n; i++ {
		c := slug[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !isAlphaNum && c != '-' {
			return false
		}
		if c == '-' && i > 0 && slug[i-1] == '-' {
			return false
		}
	}
	return true
}

func NormalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func NormalizeTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrInvalidTitle
	}
	if len(title) > 120 {
		title = title[:120]
	}
	return title, nil
}

// NormalizeTags lowercases, trims, dedupes, and validates tag strings.
// Returns an empty non-nil slice when tags is empty so JSON round-trips cleanly.
func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		tag = strings.ReplaceAll(tag, " ", "-")
		tag = strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r
			case r >= '0' && r <= '9':
				return r
			case r == '-' || r == '_':
				return r
			default:
				return -1
			}
		}, tag)
		tag = strings.Trim(tag, "-_")
		if tag == "" {
			continue
		}
		if len(tag) > maxTagLen {
			return nil, ErrInvalidTags
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) > maxTagsPerDashboard {
			return nil, ErrInvalidTags
		}
	}
	return out, nil
}

func copyTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

func copyTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

// ParseExpiresAt accepts RFC3339 or YYYY-MM-DD (end of that UTC day).
// Empty string clears the expiration (returns nil, nil).
func ParseExpiresAt(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, time.UTC); err == nil {
		end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
		return &end, nil
	}
	return nil, ErrInvalidExpires
}

func isExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}

func (s *Store) entryFromMeta(meta DashboardMeta) DashboardEntry {
	return DashboardEntry{
		Slug:      meta.Slug,
		Title:     meta.Title,
		Tags:      copyTags(meta.Tags),
		Archived:  meta.Archived,
		ExpiresAt: copyTimePtr(meta.ExpiresAt),
		CreatedAt: meta.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
		ThumbURL:  fmt.Sprintf("/api/dashboards/%s/thumb.png", meta.Slug),
		URL:       s.publicPathPrefix + "/" + meta.Slug,
	}
}

func filterByArchived(entries []DashboardEntry, archived bool) []DashboardEntry {
	out := make([]DashboardEntry, 0)
	for _, e := range entries {
		if e.Archived == archived {
			out = append(out, e)
		}
	}
	return out
}

func generateSlug() (string, error) {
	b := make([]byte, slugLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	slug := make([]byte, slugLength)
	for i, v := range b {
		slug[i] = slugChars[int(v)%len(slugChars)]
	}
	return string(slug), nil
}

func (s *Store) uniqueSlug() (string, error) {
	for i := 0; i < 20; i++ {
		slug, err := generateSlug()
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(s.dashboardDir(slug)); os.IsNotExist(err) {
			return slug, nil
		}
	}
	return "", errors.New("failed to generate unique slug")
}

func (s *Store) SlugAvailable(slug string, exceptSlug string) (bool, error) {
	slug = NormalizeSlug(slug)
	if !ValidSlug(slug) {
		return false, ErrInvalidSlug
	}
	if exceptSlug != "" && slug == NormalizeSlug(exceptSlug) {
		return true, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.dashboardDir(slug)); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func (s *Store) UpdateMeta(currentSlug, newSlug, title string, tags []string, updateTags bool, expiresAt *time.Time, updateExpires bool) (DashboardEntry, error) {
	currentSlug = NormalizeSlug(currentSlug)
	newSlug = NormalizeSlug(newSlug)
	if !ValidSlug(currentSlug) {
		return DashboardEntry{}, ErrInvalidSlug
	}

	title, err := NormalizeTitle(title)
	if err != nil {
		return DashboardEntry{}, err
	}
	var normalizedTags []string
	if updateTags {
		normalizedTags, err = NormalizeTags(tags)
		if err != nil {
			return DashboardEntry{}, err
		}
	}
	if newSlug == "" {
		newSlug = currentSlug
	}
	if !ValidSlug(newSlug) {
		return DashboardEntry{}, ErrInvalidSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.dashboardDir(currentSlug)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return DashboardEntry{}, ErrNotFound
	}

	if newSlug != currentSlug {
		if _, err := os.Stat(s.dashboardDir(newSlug)); err == nil {
			return DashboardEntry{}, ErrSlugTaken
		} else if !os.IsNotExist(err) {
			return DashboardEntry{}, err
		}
		if err := os.Rename(dir, s.dashboardDir(newSlug)); err != nil {
			return DashboardEntry{}, fmt.Errorf("rename dashboard: %w", err)
		}
		dir = s.dashboardDir(newSlug)
	}

	existing, err := s.readMetaUnlocked(newSlug)
	if err != nil {
		// meta may still be under old path naming if rename failed partially; try current
		existing, err = s.readMetaUnlocked(currentSlug)
		if err != nil {
			return DashboardEntry{}, err
		}
	}

	if !updateTags {
		normalizedTags = copyTags(existing.Tags)
	}

	now := time.Now().UTC()
	nextExpires := copyTimePtr(existing.ExpiresAt)
	if updateExpires {
		nextExpires = copyTimePtr(expiresAt)
	}

	archived := existing.Archived
	if !archived && isExpired(nextExpires, now) {
		archived = true
	}

	meta := DashboardMeta{
		Slug:             newSlug,
		Title:            title,
		Tags:             normalizedTags,
		Archived:         archived,
		ExpiresAt:        nextExpires,
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now,
		OriginalFilename: existing.OriginalFilename,
		SizeBytes:        existing.SizeBytes,
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return DashboardEntry{}, err
	}
	if err := writeFileAtomic(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		return DashboardEntry{}, err
	}

	entry := s.entryFromMeta(meta)

	entries, err := s.readManifest()
	if err != nil {
		return DashboardEntry{}, err
	}
	found := false
	for i, e := range entries {
		if e.Slug == currentSlug || e.Slug == newSlug {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}
	if err := s.writeManifest(entries); err != nil {
		return DashboardEntry{}, err
	}

	return entry, nil
}

func (s *Store) SetArchived(slug string, archived bool) (DashboardEntry, error) {
	slug = NormalizeSlug(slug)
	if !ValidSlug(slug) {
		return DashboardEntry{}, ErrInvalidSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.dashboardDir(slug)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return DashboardEntry{}, ErrNotFound
	}

	existing, err := s.readMetaUnlocked(slug)
	if err != nil {
		return DashboardEntry{}, err
	}

	now := time.Now().UTC()
	meta := existing
	meta.Archived = archived
	meta.UpdatedAt = now
	// Unarchiving an already-expired dashboard clears the date so it does not
	// immediately re-archive on the next list.
	if !archived && isExpired(meta.ExpiresAt, now) {
		meta.ExpiresAt = nil
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return DashboardEntry{}, err
	}
	if err := writeFileAtomic(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		return DashboardEntry{}, err
	}

	entry := s.entryFromMeta(meta)

	entries, err := s.readManifest()
	if err != nil {
		return DashboardEntry{}, err
	}
	found := false
	for i, e := range entries {
		if e.Slug == slug {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}
	if err := s.writeManifest(entries); err != nil {
		return DashboardEntry{}, err
	}

	return entry, nil
}

// archiveExpiredUnlocked marks expired active dashboards as archived and
// persists meta + manifest. Caller must hold s.mu.
func (s *Store) archiveExpiredUnlocked(entries []DashboardEntry) ([]DashboardEntry, error) {
	now := time.Now().UTC()
	changed := false
	for i, e := range entries {
		if e.Archived || !isExpired(e.ExpiresAt, now) {
			continue
		}
		meta, err := s.readMetaUnlocked(e.Slug)
		if err != nil {
			continue
		}
		meta.Archived = true
		meta.UpdatedAt = now
		metaData, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := writeFileAtomic(filepath.Join(s.dashboardDir(e.Slug), "meta.json"), metaData, 0o644); err != nil {
			return nil, err
		}
		entries[i] = s.entryFromMeta(meta)
		changed = true
	}
	if changed {
		if err := s.writeManifest(entries); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func titleFromFilename(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" {
		return "Untitled Dashboard"
	}
	return base
}

func (s *Store) readManifest() ([]DashboardEntry, error) {
	data, err := os.ReadFile(s.manifestPath())
	if err != nil {
		return nil, err
	}
	var entries []DashboardEntry
	if len(data) == 0 {
		return []DashboardEntry{}, nil
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) writeManifest(entries []DashboardEntry) error {
	sort.Slice(entries, func(i, j int) bool {
		ti, tj := entries[i].CreatedAt, entries[j].CreatedAt
		if !entries[i].UpdatedAt.IsZero() {
			ti = entries[i].UpdatedAt
		}
		if !entries[j].UpdatedAt.IsZero() {
			tj = entries[j].UpdatedAt
		}
		return ti.After(tj)
	})
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.manifestPath(), data, 0o644)
}

func (s *Store) List() ([]DashboardEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	entries, err = s.archiveExpiredUnlocked(entries)
	if err != nil {
		return nil, err
	}
	return filterByArchived(entries, false), nil
}

func (s *Store) ListArchived() ([]DashboardEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	entries, err = s.archiveExpiredUnlocked(entries)
	if err != nil {
		return nil, err
	}
	return filterByArchived(entries, true), nil
}

func (s *Store) ListByTag(tag string) ([]DashboardEntry, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	if tag == "" {
		return entries, nil
	}
	filtered := make([]DashboardEntry, 0, len(entries))
	for _, e := range entries {
		for _, t := range e.Tags {
			if t == tag {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered, nil
}

func (s *Store) ListTags() ([]string, error) {
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	tags := make([]string, 0)
	for _, e := range entries {
		for _, t := range e.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	sort.Strings(tags)
	return tags, nil
}

func (s *Store) SaveUpload(originalFilename string, html io.Reader, htmlSize int64, thumb io.Reader, maxHTML, maxThumb int64) (DashboardEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	slug, err := s.uniqueSlug()
	if err != nil {
		return DashboardEntry{}, err
	}

	dir := s.dashboardDir(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DashboardEntry{}, fmt.Errorf("create dashboard dir: %w", err)
	}

	htmlPath := filepath.Join(dir, "index.html")
	if err := writeStreamAtomic(htmlPath, html, 0o644, maxHTML); err != nil {
		os.RemoveAll(dir)
		if errors.Is(err, ErrTooLarge) {
			return DashboardEntry{}, err
		}
		return DashboardEntry{}, fmt.Errorf("write html: %w", err)
	}

	thumbPath := filepath.Join(dir, "thumb.png")
	if err := writeStreamAtomic(thumbPath, thumb, 0o644, maxThumb); err != nil {
		os.RemoveAll(dir)
		if errors.Is(err, ErrTooLarge) {
			return DashboardEntry{}, err
		}
		return DashboardEntry{}, fmt.Errorf("write thumb: %w", err)
	}

	now := time.Now().UTC()
	meta := DashboardMeta{
		Slug:             slug,
		Title:            titleFromFilename(originalFilename),
		Tags:             []string{},
		CreatedAt:        now,
		UpdatedAt:        now,
		OriginalFilename: originalFilename,
		SizeBytes:        htmlSize,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, err
	}
	if err := writeFileAtomic(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, err
	}

	entry := s.entryFromMeta(meta)

	entries, err := s.readManifest()
	if err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, err
	}
	entries = append(entries, entry)
	if err := s.writeManifest(entries); err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, err
	}

	return entry, nil
}

func (s *Store) ReplaceUpload(slug, originalFilename string, html io.Reader, htmlSize int64, thumb io.Reader, maxHTML, maxThumb int64) (DashboardEntry, error) {
	if !ValidSlug(slug) {
		return DashboardEntry{}, ErrInvalidSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.dashboardDir(slug)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return DashboardEntry{}, ErrNotFound
	}

	existing, err := s.readMetaUnlocked(slug)
	if err != nil {
		return DashboardEntry{}, err
	}

	htmlPath := filepath.Join(dir, "index.html")
	if err := writeStreamAtomic(htmlPath, html, 0o644, maxHTML); err != nil {
		if errors.Is(err, ErrTooLarge) {
			return DashboardEntry{}, err
		}
		return DashboardEntry{}, fmt.Errorf("write html: %w", err)
	}

	thumbPath := filepath.Join(dir, "thumb.png")
	if err := writeStreamAtomic(thumbPath, thumb, 0o644, maxThumb); err != nil {
		if errors.Is(err, ErrTooLarge) {
			return DashboardEntry{}, err
		}
		return DashboardEntry{}, fmt.Errorf("write thumb: %w", err)
	}

	now := time.Now().UTC()
	meta := DashboardMeta{
		Slug:             slug,
		Title:            titleFromFilename(originalFilename),
		Tags:             copyTags(existing.Tags),
		Archived:         existing.Archived,
		ExpiresAt:        copyTimePtr(existing.ExpiresAt),
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now,
		OriginalFilename: originalFilename,
		SizeBytes:        htmlSize,
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	if !meta.Archived && isExpired(meta.ExpiresAt, now) {
		meta.Archived = true
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return DashboardEntry{}, err
	}
	if err := writeFileAtomic(filepath.Join(dir, "meta.json"), metaData, 0o644); err != nil {
		return DashboardEntry{}, err
	}

	entry := s.entryFromMeta(meta)

	entries, err := s.readManifest()
	if err != nil {
		return DashboardEntry{}, err
	}
	found := false
	for i, e := range entries {
		if e.Slug == slug {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, entry)
	}
	if err := s.writeManifest(entries); err != nil {
		return DashboardEntry{}, err
	}

	return entry, nil
}

func (s *Store) GetMeta(slug string) (DashboardMeta, error) {
	if !ValidSlug(slug) {
		return DashboardMeta{}, ErrInvalidSlug
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readMetaUnlocked(slug)
}

func (s *Store) readMetaUnlocked(slug string) (DashboardMeta, error) {
	data, err := os.ReadFile(filepath.Join(s.dashboardDir(slug), "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return DashboardMeta{}, ErrNotFound
		}
		return DashboardMeta{}, err
	}
	var meta DashboardMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return DashboardMeta{}, err
	}
	return meta, nil
}

func (s *Store) Delete(slug string) error {
	if !ValidSlug(slug) {
		return ErrInvalidSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.dashboardDir(slug)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return ErrNotFound
	}

	entries, err := s.readManifest()
	if err != nil {
		return err
	}

	found := false
	filtered := make([]DashboardEntry, 0, len(entries))
	for _, e := range entries {
		if e.Slug == slug {
			found = true
			continue
		}
		filtered = append(filtered, e)
	}
	if !found {
		return ErrNotFound
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove dashboard: %w", err)
	}
	return s.writeManifest(filtered)
}

func (s *Store) HTMLPath(slug string) (string, error) {
	if !ValidSlug(slug) {
		return "", ErrInvalidSlug
	}
	path := filepath.Join(s.dashboardDir(slug), "index.html")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ErrNotFound
	}
	return path, nil
}

// AccessibleHTMLPath returns the HTML path only for active (non-archived) dashboards.
// Expired dashboards are archived first, then treated as inaccessible.
func (s *Store) AccessibleHTMLPath(slug string) (string, error) {
	slug = NormalizeSlug(slug)
	if !ValidSlug(slug) {
		return "", ErrInvalidSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.readMetaUnlocked(slug)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if !meta.Archived && isExpired(meta.ExpiresAt, now) {
		meta.Archived = true
		meta.UpdatedAt = now
		metaData, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return "", err
		}
		if err := writeFileAtomic(filepath.Join(s.dashboardDir(slug), "meta.json"), metaData, 0o644); err != nil {
			return "", err
		}
		entry := s.entryFromMeta(meta)
		entries, err := s.readManifest()
		if err != nil {
			return "", err
		}
		for i, e := range entries {
			if e.Slug == slug {
				entries[i] = entry
				break
			}
		}
		if err := s.writeManifest(entries); err != nil {
			return "", err
		}
	}

	if meta.Archived {
		return "", ErrArchived
	}

	path := filepath.Join(s.dashboardDir(slug), "index.html")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ErrNotFound
	}
	return path, nil
}

func (s *Store) ThumbPath(slug string) (string, error) {
	if !ValidSlug(slug) {
		return "", ErrInvalidSlug
	}
	path := filepath.Join(s.dashboardDir(slug), "thumb.png")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ErrNotFound
	}
	return path, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeStreamAtomic(path string, r io.Reader, perm os.FileMode, maxBytes int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	reader := r
	if maxBytes > 0 {
		reader = &io.LimitedReader{R: r, N: maxBytes + 1}
	}
	written, err := io.Copy(tmp, reader)
	if err != nil {
		tmp.Close()
		return err
	}
	if maxBytes > 0 && written > maxBytes {
		tmp.Close()
		return ErrTooLarge
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
