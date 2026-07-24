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

var ErrNotFound = errors.New("dashboard not found")
var ErrInvalidSlug = errors.New("invalid slug")

type DashboardMeta struct {
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	OriginalFilename string    `json:"original_filename"`
	SizeBytes        int64     `json:"size_bytes"`
}

type DashboardEntry struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ThumbURL  string    `json:"thumb_url"`
	URL       string    `json:"url"`
}

// MarshalJSON omits zero updated_at values (time.Time omitempty is unreliable).
func (e DashboardEntry) MarshalJSON() ([]byte, error) {
	type entryJSON struct {
		Slug      string     `json:"slug"`
		Title     string     `json:"title"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt *time.Time `json:"updated_at,omitempty"`
		ThumbURL  string     `json:"thumb_url"`
		URL       string     `json:"url"`
	}
	out := entryJSON{
		Slug:      e.Slug,
		Title:     e.Title,
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
	dataDir string
	mu      sync.Mutex
}

func New(dataDir string) (*Store, error) {
	dashboardsDir := filepath.Join(dataDir, "dashboards")
	if err := os.MkdirAll(dashboardsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dashboards dir: %w", err)
	}

	s := &Store{dataDir: dataDir}
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
	if len(slug) != slugLength {
		return false
	}
	for _, c := range slug {
		if !strings.ContainsRune(slugChars, c) {
			return false
		}
	}
	return true
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
	return s.readManifest()
}

func (s *Store) SaveUpload(originalFilename string, html io.Reader, htmlSize int64, thumb io.Reader) (DashboardEntry, error) {
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
	if err := writeStreamAtomic(htmlPath, html, 0o644); err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, fmt.Errorf("write html: %w", err)
	}

	thumbPath := filepath.Join(dir, "thumb.png")
	if err := writeStreamAtomic(thumbPath, thumb, 0o644); err != nil {
		os.RemoveAll(dir)
		return DashboardEntry{}, fmt.Errorf("write thumb: %w", err)
	}

	now := time.Now().UTC()
	meta := DashboardMeta{
		Slug:             slug,
		Title:            titleFromFilename(originalFilename),
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

	entry := DashboardEntry{
		Slug:      slug,
		Title:     meta.Title,
		CreatedAt: now,
		UpdatedAt: now,
		ThumbURL:  fmt.Sprintf("/api/dashboards/%s/thumb.png", slug),
		URL:       fmt.Sprintf("/d/%s", slug),
	}

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

func (s *Store) ReplaceUpload(slug, originalFilename string, html io.Reader, htmlSize int64, thumb io.Reader) (DashboardEntry, error) {
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
	if err := writeStreamAtomic(htmlPath, html, 0o644); err != nil {
		return DashboardEntry{}, fmt.Errorf("write html: %w", err)
	}

	thumbPath := filepath.Join(dir, "thumb.png")
	if err := writeStreamAtomic(thumbPath, thumb, 0o644); err != nil {
		return DashboardEntry{}, fmt.Errorf("write thumb: %w", err)
	}

	now := time.Now().UTC()
	meta := DashboardMeta{
		Slug:             slug,
		Title:            titleFromFilename(originalFilename),
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now,
		OriginalFilename: originalFilename,
		SizeBytes:        htmlSize,
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

	entry := DashboardEntry{
		Slug:      slug,
		Title:     meta.Title,
		CreatedAt: meta.CreatedAt,
		UpdatedAt: now,
		ThumbURL:  fmt.Sprintf("/api/dashboards/%s/thumb.png", slug),
		URL:       fmt.Sprintf("/d/%s", slug),
	}

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

func writeStreamAtomic(path string, r io.Reader, perm os.FileMode) error {
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

	if _, err := io.Copy(tmp, r); err != nil {
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
