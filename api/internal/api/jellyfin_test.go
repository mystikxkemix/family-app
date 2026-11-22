package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"familyapp/internal/config"
	"familyapp/internal/store"
)

func init() {
	// Tests run the sync themselves (POST /admin/jellyfin/sync).
	jellyfinAutoSync = false
}

// fakeJellyfin is a Jellyfin server whose library a test can change.
type fakeJellyfin struct {
	mu       sync.Mutex
	items    map[string]string // id → item JSON
	episodes []string
}

func (f *fakeJellyfin) set(id, item string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if item == "" {
		delete(f.items, id)
		return
	}
	f.items[id] = item
}

func newFakeJellyfin(t *testing.T) (*fakeJellyfin, *httptest.Server) {
	t.Helper()
	fake := &fakeJellyfin{items: map[string]string{
		// In the watchlist with its TMDB id: played → Watched.
		"m1": `{"Id":"m1","Name":"Arrival","Type":"Movie","ProductionYear":2016,"ProviderIds":{"Tmdb":"329865"},"UserData":{"Played":true,"LastPlayedDate":"2025-03-22T20:00:00Z"}}`,
		// In the watchlist without ids: found by title + year, not played.
		"m2": `{"Id":"m2","Name":"Dune","Type":"Movie","ProductionYear":2021,"ProviderIds":{"Tmdb":"438631","Tvdb":"1111"},"UserData":{"Played":false}}`,
		// Same title, other year: never taken for the 2021 one.
		"m3": `{"Id":"m3","Name":"Dune","Type":"Movie","ProductionYear":1984,"ProviderIds":{"Tmdb":"841"},"UserData":{"Played":false}}`,
		// In the watchlist with its TVDB id: partly played → Watching.
		"s1": `{"Id":"s1","Name":"Severance","Type":"Series","ProductionYear":2022,"ProviderIds":{"Tmdb":"95396","Tvdb":"371980"},"UserData":{"Played":false,"PlayedPercentage":50}}`,
		// Not in the watchlist, played → imported.
		"m4": `{"Id":"m4","Name":"Heat","Type":"Movie","ProductionYear":1995,"ProviderIds":{"Tmdb":"949"},"UserData":{"Played":true,"LastPlayedDate":"2024-05-01T20:00:00Z"}}`,
		"s2": `{"Id":"s2","Name":"Dark","Type":"Series","ProductionYear":2017,"ProviderIds":{"Tmdb":"70523","Tvdb":"334824"},"UserData":{"Played":true}}`,
		// Not in the watchlist, partly played → not imported.
		"s3": `{"Id":"s3","Name":"Halfway","Type":"Series","ProviderIds":{"Tvdb":"5"},"UserData":{"Played":false,"PlayedPercentage":10}}`,
		// No TMDB or TVDB id (a YouTube download): skipped.
		"y1": `{"Id":"y1","Name":"Some channel","Type":"Series","ProviderIds":{},"UserData":{"Played":true}}`,
	}, episodes: []string{
		`{"Id":"e1","Type":"Episode","SeriesId":"s2","UserData":{"Played":true,"LastPlayedDate":"2024-02-01T10:00:00Z"}}`,
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != `MediaBrowser Token="jf-key"` {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("the sync must only read Jellyfin: %s %s", r.Method, r.URL.Path)
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"Version":"12.4.0"}`))
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"Home"},{"Id":"u2","Name":"Guest"}]`))
		case "/Items":
			if r.URL.Query().Get("userId") != "u1" {
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
				return
			}
			list := fake.episodes
			if r.URL.Query().Get("IncludeItemTypes") != "Episode" {
				list = make([]string, 0, len(fake.items))
				for _, item := range fake.items {
					list = append(list, item)
				}
			}
			_, _ = w.Write([]byte(`{"Items":[` + strings.Join(list, ",") + `],"TotalRecordCount":` + strconv.Itoa(len(list)) + `}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return fake, server
}

func TestJellyfinSync(t *testing.T) {
	fake, jellyfinServer := newFakeJellyfin(t)
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	settings := config.Settings{Jellyfin: config.JellyfinConfig{URL: jellyfinServer.URL, APIKey: "jf-key"}}
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Settings: settings, SettingsFile: settingsFile})
	alex := signUp(t, server, httpServer.URL)

	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.Integrations.Jellyfin {
		t.Fatal("integrations.jellyfin must be true")
	}

	// The watchlist before the first sync.
	var arrival, dune, severance, notThere store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Arrival", "externalIds": map[string]int{"tmdb": 329865}}, 201, &arrival)
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Dune", "year": 2021}, 201, &dune)
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "show", "title": "Severance", "externalIds": map[string]int{"tvdb": 371980}}, 201, &severance)
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Not in Jellyfin"}, 201, &notThere)

	// Not linked yet: the sync reads nothing.
	var result jellyfinSyncResult
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if !result.OK || result.Households != 0 || result.Imported != 0 {
		t.Fatalf("sync without a linked household: %+v", result)
	}

	// Link the household to a Jellyfin account.
	var users []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	alex.call("GET", "/api/v1/admin/jellyfin/users", nil, 200, &users)
	if len(users) != 2 || users[0].ID != "u1" {
		t.Fatalf("users: %+v", users)
	}
	alex.call("PUT", "/api/v1/admin/jellyfin/household", map[string]string{"jellyfinUserId": "nobody"}, 400, nil)
	var state adminJellyfinState
	alex.call("PUT", "/api/v1/admin/jellyfin/household", map[string]string{"jellyfinUserId": "u1"}, 200, &state)
	if !state.Configured || state.JellyfinUserID != "u1" || state.LastSync == nil {
		t.Fatalf("state: %+v", state)
	}

	// First sync.
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if !result.OK || result.Households != 1 || result.Available != 7 || result.Matched != 3 ||
		result.MarkedWatched != 1 || result.MarkedWatching != 1 || result.Imported != 2 {
		t.Fatalf("first sync: %+v", result)
	}
	titles := watchlistByTitle(t, alex)
	if got := titles["Arrival"]; got.Status != store.MediaStatusWatched || got.WatchedAt == nil || got.WatchedAt.Year() != 2025 ||
		got.Jellyfin == nil || !got.Jellyfin.Available || got.Jellyfin.PlayedAt == nil {
		t.Fatalf("Arrival: %+v %+v", got, got.Jellyfin)
	}
	if got := titles["Dune"]; got.Status != store.MediaStatusToWatch || got.Jellyfin == nil || !got.Jellyfin.Available ||
		got.ExternalIDs.TMDB != 438631 || got.ExternalIDs.TVDB != 1111 {
		t.Fatalf("Dune (2021, by title + year): %+v", got)
	}
	if got := titles["Severance"]; got.Status != store.MediaStatusWatching || got.ExternalIDs.TMDB != 95396 {
		t.Fatalf("Severance: %+v", got)
	}
	if got := titles["Not in Jellyfin"]; got.Jellyfin != nil {
		t.Fatalf("no match expected: %+v", got)
	}
	heat, dark := titles["Heat"], titles["Dark"]
	if heat.Source != store.SourceJellyfin || heat.Status != store.MediaStatusWatched || len(heat.InterestedUserIDs) != 0 ||
		heat.ExternalIDs.TMDB != 949 || heat.WatchedAt == nil || heat.WatchedAt.Year() != 2024 {
		t.Fatalf("imported movie: %+v", heat)
	}
	if dark.Kind != store.MediaKindShow || dark.Source != store.SourceJellyfin || dark.WatchedAt == nil || dark.WatchedAt.Month() != time.February {
		t.Fatalf("imported show (date of its last episode): %+v", dark)
	}
	if _, ok := titles["Halfway"]; ok {
		t.Fatal("a partly played show must not be imported")
	}
	if _, ok := titles["Some channel"]; ok {
		t.Fatal("items without TMDB or TVDB id must be skipped")
	}

	// The same sync again changes nothing and imports nothing twice.
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if result.Imported != 0 || result.MarkedWatched != 0 || len(watchlistByTitle(t, alex)) != 6 {
		t.Fatalf("second sync: %+v", result)
	}

	// A user's status wins while Jellyfin says the same thing...
	alex.call("PATCH", "/api/v1/watchlist/"+arrival.ID.Hex(), map[string]string{"status": "toWatch"}, 200, nil)
	alex.call("PATCH", "/api/v1/watchlist/"+severance.ID.Hex(), map[string]string{"status": "toWatch"}, 200, nil)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	titles = watchlistByTitle(t, alex)
	if titles["Arrival"].Status != store.MediaStatusToWatch || titles["Severance"].Status != store.MediaStatusToWatch {
		t.Fatalf("the user's choice must be kept: %+v", result)
	}
	// ...and Jellyfin moves it forward again when its state changes.
	fake.set("s1", `{"Id":"s1","Name":"Severance","Type":"Series","ProductionYear":2022,"ProviderIds":{"Tvdb":"371980"},"UserData":{"Played":true,"PlayedPercentage":100}}`)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if got := watchlistByTitle(t, alex)["Severance"]; got.Status != store.MediaStatusWatched {
		t.Fatalf("Severance fully played: %+v", got)
	}
	// The sync never moves a status back.
	alex.call("PATCH", "/api/v1/watchlist/"+dune.ID.Hex(), map[string]string{"status": "watched"}, 200, nil)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if got := watchlistByTitle(t, alex)["Dune"]; got.Status != store.MediaStatusWatched {
		t.Fatalf("Dune must stay watched: %+v", got)
	}

	// A deleted import is not imported again.
	alex.call("DELETE", "/api/v1/watchlist/"+heat.ID.Hex(), nil, 204, nil)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if _, ok := watchlistByTitle(t, alex)["Heat"]; ok || result.Imported != 0 {
		t.Fatalf("Heat was deleted on purpose: %+v", result)
	}

	// Removed from Jellyfin: no longer available.
	fake.set("m2", "")
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if got := watchlistByTitle(t, alex)["Dune"]; got.Jellyfin == nil || got.Jellyfin.Available || result.Gone != 1 {
		t.Fatalf("Dune removed from Jellyfin: %+v %+v", got.Jellyfin, result)
	}

	// One sync at a time.
	server.jellyfinRunning.Store(true)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 409, nil)
	alex.call("GET", "/api/v1/admin/jellyfin", nil, 200, &state)
	if !state.Running {
		t.Fatal("state must say a sync is running")
	}
	server.jellyfinRunning.Store(false)

	// Another household sees none of it.
	stranger := strangerClient(t, server, httpServer.URL)
	var strangerItems []store.Media
	stranger.call("GET", "/api/v1/watchlist", nil, 200, &strangerItems)
	if len(strangerItems) != 0 {
		t.Fatalf("household isolation: %+v", strangerItems)
	}
	stranger.call("POST", "/api/v1/admin/jellyfin/sync", nil, 403, nil)
	stranger.call("GET", "/api/v1/admin/jellyfin/users", nil, 403, nil)

	// Connections and the Test button.
	var results []connectionResult
	alex.call("GET", "/api/v1/admin/connections", nil, 200, &results)
	found := false
	for _, row := range results {
		if row.Service == "jellyfin" {
			found = row.OK && row.Message == "Jellyfin 12.4.0"
		}
	}
	if !found {
		t.Fatalf("connections: %+v", results)
	}
	var test testToolResponse
	alex.call("POST", "/api/v1/admin/settings/test", map[string]string{"tool": "jellyfin"}, 200, &test)
	if !test.OK {
		t.Fatalf("test: %+v", test)
	}

	// Unlink: the sync reads nothing for this household.
	alex.call("PUT", "/api/v1/admin/jellyfin/household", map[string]string{"jellyfinUserId": ""}, 200, &state)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if state.JellyfinUserID != "" || result.Households != 0 {
		t.Fatalf("unlinked: %+v %+v", state, result)
	}
}

func TestJellyfinSettings(t *testing.T) {
	_, jellyfinServer := newFakeJellyfin(t)
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, SettingsFile: settingsFile})
	alex := signUp(t, server, httpServer.URL)

	// Not set up.
	alex.call("GET", "/api/v1/admin/jellyfin/users", nil, 503, nil)
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 503, nil)
	alex.call("PUT", "/api/v1/admin/jellyfin/household", map[string]string{"jellyfinUserId": "u1"}, 503, nil)

	body := map[string]any{"radarr": map[string]any{"url": ""}, "sonarr": map[string]any{"url": ""}, "prowlarr": map[string]any{"url": ""},
		"tmdb": map[string]any{}, "apns": map[string]any{"credentials": []any{}},
		"jellyfin": map[string]any{"url": jellyfinServer.URL + "/", "apiKey": "jf-key"}}
	var saved adminSettings
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if saved.Jellyfin.URL != jellyfinServer.URL || !saved.Jellyfin.APIKeySet || saved.Jellyfin.APIKey != "" {
		t.Fatalf("jellyfin settings: %+v", saved.Jellyfin)
	}
	alex.call("GET", "/api/v1/admin/jellyfin/users", nil, 200, nil)

	// An empty key keeps the saved one; no key at all is refused.
	body["jellyfin"] = map[string]any{"url": jellyfinServer.URL}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if !saved.Jellyfin.APIKeySet {
		t.Fatal("an empty key must keep the saved one")
	}
	body["jellyfin"] = map[string]any{"url": "ftp://x"}
	alex.call("PUT", "/api/v1/admin/settings", body, 400, nil)

	// A wrong key: a clear error, never the key itself.
	body["jellyfin"] = map[string]any{"url": jellyfinServer.URL, "apiKey": "wrong-key"}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, nil)
	alex.call("GET", "/api/v1/admin/jellyfin/users", nil, 502, nil)
	var test testToolResponse
	alex.call("POST", "/api/v1/admin/settings/test", map[string]string{"tool": "jellyfin"}, 200, &test)
	if test.OK || !strings.Contains(test.Message, "not valid") || strings.Contains(test.Message, "wrong-key") {
		t.Fatalf("wrong key: %+v", test)
	}

	// An empty URL turns Jellyfin off.
	body["jellyfin"] = map[string]any{"url": ""}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if saved.Jellyfin.APIKeySet || me.Integrations.Jellyfin {
		t.Fatal("an empty URL must turn Jellyfin off")
	}
}

func TestJellyfinImportPosters(t *testing.T) {
	fake, jellyfinServer := newFakeJellyfin(t)
	// Only two played titles: one TMDB knows, one it does not.
	fake.mu.Lock()
	fake.items = map[string]string{
		"m1": `{"Id":"m1","Name":"Arrival","Type":"Movie","ProductionYear":2016,"ProviderIds":{"Tmdb":"329865"},"UserData":{"Played":true}}`,
		"m4": `{"Id":"m4","Name":"Heat","Type":"Movie","ProductionYear":1995,"ProviderIds":{"Tmdb":"949"},"UserData":{"Played":true}}`,
	}
	fake.mu.Unlock()
	tmdbServer := fakeTMDB(t, nil)
	matchDelay = time.Millisecond
	settings := config.Settings{
		Jellyfin: config.JellyfinConfig{URL: jellyfinServer.URL, APIKey: "jf-key"},
		TMDB:     config.TMDBConfig{APIKey: "tmdb-key"},
	}
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Settings: settings, TMDBBaseURL: tmdbServer.URL})
	alex := signUp(t, server, httpServer.URL)
	alex.call("PUT", "/api/v1/admin/jellyfin/household", map[string]string{"jellyfinUserId": "u1"}, 200, nil)
	var result jellyfinSyncResult
	alex.call("POST", "/api/v1/admin/jellyfin/sync", nil, 200, &result)
	if result.Imported != 2 {
		t.Fatalf("sync: %+v", result)
	}

	server.fillImportedPosters()
	titles := watchlistByTitle(t, alex)
	if got := titles["Arrival"]; got.PosterURL == "" || got.ExternalIDs.IMDB != "tt2543164" {
		t.Fatalf("the poster comes from TMDB: %+v", got)
	}
	if got := titles["Heat"]; got.PosterURL != "" {
		t.Fatalf("unknown at TMDB: no poster: %+v", got)
	}
	// Each title is looked up once.
	pending, err := server.store.ImportedWithoutDetails(t.Context(), store.SourceJellyfin, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending: %+v %v", pending, err)
	}
}

// watchlistByTitle reads the whole watchlist (fresh values, keyed by title).
func watchlistByTitle(t *testing.T, c *client) map[string]store.Media {
	t.Helper()
	var items []store.Media
	c.call("GET", "/api/v1/watchlist", nil, 200, &items)
	byTitle := make(map[string]store.Media, len(items))
	for _, item := range items {
		byTitle[item.Title] = item
	}
	return byTitle
}
