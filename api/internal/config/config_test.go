package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSettings(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")

	settings, found, err := LoadSettings(path)
	if err != nil || found || settings.Radarr.Enabled() {
		t.Fatalf("a missing file must give empty settings: found=%v err=%v", found, err)
	}

	content := `{
	  "apns": {"credentials": [
	    {"teamId": "T1", "keyId": "K1", "keyFile": "apns/AuthKey_K1.p8", "topics": ["a.b"]},
	    {"teamId": "T2", "keyId": "K2", "keyFile": "/abs/AuthKey_K2.p8", "topics": ["c.d"]}
	  ]},
	  "radarr": {"url": "http://radarr:7878", "apiKey": "r", "qualityProfileId": 4},
	  "prowlarr": {"url": "http://prowlarr:9696", "apiKey": "p"}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, found, err = LoadSettings(path)
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if !settings.Radarr.Enabled() || settings.Radarr.QualityProfileID != 4 || settings.Sonarr.Enabled() || !settings.Prowlarr.Enabled() {
		t.Fatalf("bad tools: %+v", settings)
	}
	keys := settings.APNs.Credentials
	if keys[0].KeyFile != filepath.Join(directory, "apns/AuthKey_K1.p8") || keys[1].KeyFile != "/abs/AuthKey_K2.p8" {
		t.Fatalf("key paths must be relative to the settings file: %+v", keys)
	}

	if err := os.WriteFile(path, []byte(`{"radar": {"url": "x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadSettings(path); err == nil || !strings.Contains(err.Error(), "radar") {
		t.Fatalf("an unknown key must fail, got %v", err)
	}
}
