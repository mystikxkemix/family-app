package arr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"familyapp/internal/config"
	"familyapp/internal/store"
)

// fakeArr imitates the Radarr, Sonarr and Prowlarr endpoints we use.
type fakeArr struct {
	mu      sync.Mutex
	posted  map[string]any // last body posted to /api/v3/movie or /series
	grabbed map[string]any
	query   string
}

func (f *fakeArr) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }
	mux.HandleFunc("GET /api/v3/movie/lookup", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("term") {
		case "tmdb:1":
			reply(w, []map[string]any{{"title": "Arrival", "year": 2016, "tmdbId": 1, "remotePoster": "https://img/p.jpg", "extra": "kept"}})
		case "tmdb:2":
			reply(w, []map[string]any{{"id": 7, "title": "Owned", "tmdbId": 2}})
		default:
			reply(w, []map[string]any{
				{"title": "Arrival", "year": 2016, "tmdbId": 1, "imdbId": "tt1", "remotePoster": "https://img/p.jpg"},
				{"id": 3, "title": "Arrival 2", "tmdbId": 9, "images": []map[string]string{{"coverType": "poster", "remoteUrl": "http://insecure/p.jpg"}}},
			})
		}
	})
	mux.HandleFunc("GET /api/v3/series/lookup", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []map[string]any{{"title": "Severance", "tvdbId": 5, "images": []map[string]string{{"coverType": "poster", "remoteUrl": "https://img/s.jpg"}}}})
	})
	mux.HandleFunc("GET /api/v3/qualityprofile", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []map[string]any{{"id": 4}, {"id": 6}})
	})
	mux.HandleFunc("GET /api/v3/rootfolder", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []map[string]any{{"path": "/movies"}})
	})
	post := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&f.posted)
		w.WriteHeader(http.StatusCreated)
		reply(w, map[string]any{"id": 99})
	}
	mux.HandleFunc("POST /api/v3/movie", post)
	mux.HandleFunc("POST /api/v3/series", post)
	mux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.query = r.URL.RawQuery
		f.mu.Unlock()
		if r.URL.Query().Get("query") == "boom" {
			w.WriteHeader(http.StatusBadRequest)
			reply(w, map[string]string{"message": "indexer down"})
			return
		}
		reply(w, []map[string]any{{"guid": "g1", "indexerId": 2, "indexer": "Idx", "title": "Arrival.2016.1080p", "size": 123, "seeders": 10, "protocol": "torrent", "publishDate": "2020-01-02T03:04:05Z"}})
	})
	mux.HandleFunc("POST /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&f.grabbed)
		reply(w, map[string]any{})
	})
	return mux
}

func newFake(t *testing.T) (*fakeArr, *Services) {
	fake := &fakeArr{}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	cfg := config.ArrConfig{URL: server.URL + "/", APIKey: "secret"}
	return fake, New(cfg, cfg, cfg)
}

func TestSearchMapsResults(t *testing.T) {
	_, services := newFake(t)
	results, err := services.Radarr.Search(context.Background(), "arrival")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %+v", results)
	}
	first, second := results[0], results[1]
	if first.Kind != store.MediaKindMovie || first.ExternalIDs.TMDB != 1 || first.ExternalIDs.IMDB != "tt1" || first.PosterURL != "https://img/p.jpg" || first.InLibrary {
		t.Fatalf("bad first result: %+v", first)
	}
	if !second.InLibrary || second.PosterURL != "" {
		t.Fatalf("second result must be in library with no http poster: %+v", second)
	}
	shows, err := services.Sonarr.Search(context.Background(), "severance")
	if err != nil || len(shows) != 1 || shows[0].ExternalIDs.TVDB != 5 || shows[0].PosterURL != "https://img/s.jpg" {
		t.Fatalf("bad show search: %+v %v", shows, err)
	}
}

func TestAddPostsLookupWithDestination(t *testing.T) {
	fake, services := newFake(t)
	already, err := services.Radarr.Add(context.Background(), store.ExternalIDs{TMDB: 1})
	if err != nil || already {
		t.Fatalf("add: already=%v err=%v", already, err)
	}
	body := fake.posted
	if body["qualityProfileId"] != float64(4) || body["rootFolderPath"] != "/movies" || body["extra"] != "kept" || body["monitored"] != true {
		t.Fatalf("bad posted body: %v", body)
	}
	options, _ := body["addOptions"].(map[string]any)
	if options["searchForMovie"] != true {
		t.Fatalf("movie must be searched right away: %v", body)
	}

	fake.posted = nil
	already, err = services.Radarr.Add(context.Background(), store.ExternalIDs{TMDB: 2})
	if err != nil || !already || fake.posted != nil {
		t.Fatalf("title in library must not be posted again: already=%v err=%v", already, err)
	}

	if _, err := services.Sonarr.Add(context.Background(), store.ExternalIDs{TMDB: 1}); !errors.Is(err, ErrMissingID) {
		t.Fatalf("a show needs a TVDB id, got %v", err)
	}
	if _, err := services.Sonarr.Add(context.Background(), store.ExternalIDs{TVDB: 5}); err != nil {
		t.Fatal(err)
	}
	if fake.posted["seasonFolder"] != true {
		t.Fatalf("bad series body: %v", fake.posted)
	}
}

func TestProwlarrSearchAndGrab(t *testing.T) {
	fake, services := newFake(t)
	releases, err := services.Prowlarr.Search(context.Background(), "arrival", store.MediaKindMovie)
	if err != nil || len(releases) != 1 || releases[0].GUID != "g1" || *releases[0].Seeders != 10 {
		t.Fatalf("search: %+v %v", releases, err)
	}
	if fake.query != "categories=2000&limit=100&query=arrival&type=search" {
		t.Fatalf("bad query: %s", fake.query)
	}
	if err := services.Prowlarr.Grab(context.Background(), "g1", 2); err != nil {
		t.Fatal(err)
	}
	if fake.grabbed["guid"] != "g1" || fake.grabbed["indexerId"] != float64(2) {
		t.Fatalf("bad grab body: %v", fake.grabbed)
	}
	_, err = services.Prowlarr.Search(context.Background(), "boom", "")
	if !errors.Is(err, ErrUpstream) || err.Error() == "" {
		t.Fatalf("want upstream error, got %v", err)
	}
}

func TestDisabledTools(t *testing.T) {
	services := New(config.ArrConfig{}, config.ArrConfig{URL: "x"}, config.ArrConfig{APIKey: "y"})
	if services.Radarr != nil || services.Sonarr != nil || services.Prowlarr != nil {
		t.Fatal("tools without URL and key must be off")
	}
	if services.Library(store.MediaKindMovie) != nil {
		t.Fatal("Library must be nil when off")
	}
}
