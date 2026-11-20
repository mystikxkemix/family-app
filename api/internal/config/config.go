// Package config reads the API settings.
//
// Plain settings (port, database...) come from environment variables. Secrets
// and addresses of other services (APNs keys, Radarr, Sonarr, Prowlarr) come
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
