// Package config reads the API settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds every runtime setting of the API.
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
	// APNSConfigFile is the path of the APNs JSON config. Empty disables push.
	APNSConfigFile string
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
		APNSConfigFile: os.Getenv("APNS_CONFIG_FILE"),
	}, nil
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
