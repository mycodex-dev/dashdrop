package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultAppName = "Dashdrop"
	maxAppNameLen  = 64
	MaxLogoBytes   = 512 << 10 // 512 KiB
)

var (
	ErrInvalidAppName      = errors.New("invalid app name")
	ErrInvalidPrimaryColor = errors.New("invalid primary color")
	ErrInvalidLogo         = errors.New("invalid logo")
)

var hexColorRe = regexp.MustCompile(`(?i)^#[0-9a-f]{6}$`)

var allowedLogoExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".gif":  "image/gif",
}

// InstanceSettings holds instance-wide branding preferences.
type InstanceSettings struct {
	AppName       string `json:"app_name"`
	PrimaryColor  string `json:"primary_color"`
	LogoFilename  string `json:"logo_filename,omitempty"`
}

func (s *Store) settingsPath() string {
	return filepath.Join(s.dataDir, "settings.json")
}

func (s *Store) brandingDir() string {
	return filepath.Join(s.dataDir, "branding")
}

func DefaultSettings() InstanceSettings {
	return InstanceSettings{
		AppName: defaultAppName,
	}
}

func NormalizeAppName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxAppNameLen {
		return "", ErrInvalidAppName
	}
	return name, nil
}

func NormalizePrimaryColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if !hexColorRe.MatchString(color) {
		return "", ErrInvalidPrimaryColor
	}
	return strings.ToLower(color), nil
}

func (s *Store) GetSettings() (InstanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readSettingsUnlocked()
}

func (s *Store) readSettingsUnlocked() (InstanceSettings, error) {
	data, err := os.ReadFile(s.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return InstanceSettings{}, err
	}
	if len(data) == 0 {
		return DefaultSettings(), nil
	}
	var settings InstanceSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return InstanceSettings{}, err
	}
	return normalizeSettings(settings), nil
}

func normalizeSettings(settings InstanceSettings) InstanceSettings {
	defaults := DefaultSettings()
	if name, err := NormalizeAppName(settings.AppName); err == nil {
		settings.AppName = name
	} else {
		settings.AppName = defaults.AppName
	}
	if color, err := NormalizePrimaryColor(settings.PrimaryColor); err == nil {
		settings.PrimaryColor = color
	} else {
		settings.PrimaryColor = ""
	}
	settings.LogoFilename = filepath.Base(strings.TrimSpace(settings.LogoFilename))
	if settings.LogoFilename == "." || settings.LogoFilename == string(filepath.Separator) {
		settings.LogoFilename = ""
	}
	return settings
}

func (s *Store) SaveSettings(appName, primaryColor string) (InstanceSettings, error) {
	name, err := NormalizeAppName(appName)
	if err != nil {
		return InstanceSettings{}, err
	}
	color, err := NormalizePrimaryColor(primaryColor)
	if err != nil {
		return InstanceSettings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.readSettingsUnlocked()
	if err != nil {
		return InstanceSettings{}, err
	}
	settings.AppName = name
	settings.PrimaryColor = color
	if err := s.writeSettingsUnlocked(settings); err != nil {
		return InstanceSettings{}, err
	}
	return settings, nil
}

func (s *Store) writeSettingsUnlocked(settings InstanceSettings) error {
	settings = normalizeSettings(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.settingsPath(), data, 0o644)
}

func (s *Store) SaveLogo(filename string, r io.Reader, size int64) (InstanceSettings, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if _, ok := allowedLogoExt[ext]; !ok {
		return InstanceSettings{}, ErrInvalidLogo
	}
	if size < 0 || size > MaxLogoBytes {
		return InstanceSettings{}, ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.readSettingsUnlocked()
	if err != nil {
		return InstanceSettings{}, err
	}

	if err := os.MkdirAll(s.brandingDir(), 0o755); err != nil {
		return InstanceSettings{}, fmt.Errorf("create branding dir: %w", err)
	}

	logoName := "logo" + ext
	logoPath := filepath.Join(s.brandingDir(), logoName)
	if err := writeStreamAtomic(logoPath, r, 0o644, MaxLogoBytes); err != nil {
		return InstanceSettings{}, err
	}

	// Remove previous logo if extension changed.
	if settings.LogoFilename != "" && settings.LogoFilename != logoName {
		_ = os.Remove(filepath.Join(s.brandingDir(), settings.LogoFilename))
	}

	settings.LogoFilename = logoName
	if err := s.writeSettingsUnlocked(settings); err != nil {
		return InstanceSettings{}, err
	}
	return settings, nil
}

func (s *Store) DeleteLogo() (InstanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.readSettingsUnlocked()
	if err != nil {
		return InstanceSettings{}, err
	}
	if settings.LogoFilename != "" {
		_ = os.Remove(filepath.Join(s.brandingDir(), settings.LogoFilename))
		settings.LogoFilename = ""
		if err := s.writeSettingsUnlocked(settings); err != nil {
			return InstanceSettings{}, err
		}
	}
	return settings, nil
}

func (s *Store) LogoPath() (path string, contentType string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.readSettingsUnlocked()
	if err != nil {
		return "", "", err
	}
	if settings.LogoFilename == "" {
		return "", "", ErrNotFound
	}
	ext := strings.ToLower(filepath.Ext(settings.LogoFilename))
	ct, ok := allowedLogoExt[ext]
	if !ok {
		return "", "", ErrNotFound
	}
	path = filepath.Join(s.brandingDir(), settings.LogoFilename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", "", ErrNotFound
	}
	return path, ct, nil
}
