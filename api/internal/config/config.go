// Package config reads the API settings.
//
// Plain settings (port, database...) come from environment variables. Secrets
// and addresses of other services (APNs keys, Radarr, Sonarr, Prowlarr, TMDB,
// Jellyfin) come
// from a local JSON file, so they never show up in a container's environment.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"familyapp/internal/push"
)

// DefaultSettingsFile is where the Docker image looks for the settings file.
const DefaultSettingsFile = "/data/settings.json"

// Config holds the plain runtime settings of the API.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
	// MongoURI is the MongoDB connection string.
	MongoURI string
	// MongoDB is the database name.
	MongoDB string
	// SessionTTL is how long a login stays valid without being used.
	SessionTTL time.Duration
	// ClientIPHeader names the header that carries the real client IP when the
	// API runs behind a reverse proxy (for example X-Real-IP, CF-Connecting-IP or
	// X-Forwarded-For). Empty means "use the TCP peer address". Only set it when a
	// proxy always overwrites this header, or clients can fake their IP.
	ClientIPHeader string
	// SettingsFile is the path of the JSON settings file (see Settings).
	SettingsFile string
	// AdminUsername and AdminPassword create the first account when the
	// database has none. Its password must be changed at first login. They are
	// ignored once an account exists.
	AdminUsername string
	AdminPassword string
	// TMDBBaseURL replaces the TMDB API address. Development only (a local
	// fake TMDB); leave it empty in production.
	TMDBBaseURL string
}

// FromEnv builds the config from the environment, with defaults for local use.
func FromEnv() (Config, error) {
	ttlDays, err := intEnv("SESSION_TTL_DAYS", 180)
	if err != nil {
		return Config{}, err
	}
	if ttlDays < 1 {
		return Config{}, fmt.Errorf("SESSION_TTL_DAYS must be at least 1")
	}
	return Config{
		Port:           stringEnv("PORT", "8080"),
		MongoURI:       stringEnv("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDB:        stringEnv("MONGODB_DB", "family-app"),
		SessionTTL:     time.Duration(ttlDays) * 24 * time.Hour,
		ClientIPHeader: os.Getenv("CLIENT_IP_HEADER"),
		SettingsFile:   stringEnv("SETTINGS_FILE", DefaultSettingsFile),
		AdminUsername:  stringEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		TMDBBaseURL:    os.Getenv("TMDB_BASE_URL"),
	}, nil
}

// Settings is the content of the settings file. Every part is optional: a
// missing part (or a missing file) turns that feature off.
type Settings struct {
	// APNs holds the push keys, one per Apple team.
	APNs     push.Config `json:"apns"`
	Radarr   ArrConfig   `json:"radarr"`
	Sonarr   ArrConfig   `json:"sonarr"`
	Prowlarr ArrConfig   `json:"prowlarr"`
	// TMDB gives movie and show details (see internal/tmdb).
	TMDB TMDBConfig `json:"tmdb"`
	// Jellyfin tells what we have and what we watched (see internal/jellyfin).
	Jellyfin JellyfinConfig `json:"jellyfin"`
}

// JellyfinConfig is the address of the Jellyfin server and an API key from its
// dashboard (Dashboard > API Keys). The Jellyfin user of each household is
// chosen in the admin page and saved in the database.
type JellyfinConfig struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// Enabled reports whether Jellyfin has an address and a key.
func (c JellyfinConfig) Enabled() bool {
	return c.URL != "" && c.APIKey != ""
}

// TMDBConfig is the key of The Movie Database: a v3 API key or a v4 read
// access token.
type TMDBConfig struct {
	APIKey string `json:"apiKey"`
}

// Enabled reports whether a TMDB key is set.
func (c TMDBConfig) Enabled() bool {
	return c.APIKey != ""
}

// ArrConfig is the address and key of one *arr tool.
type ArrConfig struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
	// QualityProfileID and RootFolder choose where Radarr/Sonarr add a title.
	// Zero/empty means "the first one configured in the tool".
	QualityProfileID int    `json:"qualityProfileId,omitempty"`
	RootFolder       string `json:"rootFolder,omitempty"`
}

// Enabled reports whether the tool has an address and a key.
func (c ArrConfig) Enabled() bool {
	return c.URL != "" && c.APIKey != ""
}

// LoadSettings reads the settings file. A missing file is not an error: it
// returns empty settings and found = false.
func LoadSettings(path string) (settings Settings, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("read settings file: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	// A typo in a key must fail loudly, not silently turn a feature off.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, true, fmt.Errorf("parse settings file %s: %w", path, err)
	}
	// APNs key files may be given relative to the settings file.
	directory := filepath.Dir(path)
	for i, credential := range settings.APNs.Credentials {
		if credential.KeyFile != "" && !filepath.IsAbs(credential.KeyFile) {
			settings.APNs.Credentials[i].KeyFile = filepath.Join(directory, credential.KeyFile)
		}
	}
	return settings, true, nil
}

// ErrNotWritable means the API cannot write in the folder of the settings file.
var ErrNotWritable = errors.New("the settings folder is not writable")

// SaveSettings writes the settings file atomically (temporary file, then
// rename), readable by the API user only: it holds secrets.
func SaveSettings(path string, settings Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'))
}

// WriteFileAtomic writes a file with mode 0600 through a temporary file in the
// same folder, so a crash never leaves a half-written file.
func WriteFileAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return notWritable(err)
	}
	temporary, err := os.CreateTemp(directory, ".tmp-*")
	if err != nil {
		return notWritable(err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return notWritable(err)
	}
	return nil
}

// SettingsWritable reports whether SaveSettings can write next to path.
func SettingsWritable(path string) bool {
	directory := filepath.Dir(path)
	probe, err := os.CreateTemp(directory, ".probe-*")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

func notWritable(err error) error {
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return fmt.Errorf("%w: %v", ErrNotWritable, err)
	}
	return err
}

func stringEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", key, err)
	}
	return parsed, nil
}
