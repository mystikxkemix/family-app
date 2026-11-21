package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"familyapp/internal/store"
)

// fakeTMDB answers the few TMDB endpoints we use.
func fakeTMDB(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	reply := func(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if r.URL.Query().Get("api_key") != "key" && r.Header.Get("Authorization") != "Bearer eyJtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			reply(w, map[string]any{"status_message": "Invalid API key"})
			return
		}
		switch r.URL.Path {
		case "/configuration":
			reply(w, map[string]any{"images": map[string]any{"secure_base_url": "https://image.tmdb.org/t/p/"}})
		case "/search/movie":
			// Like TMDB, the search ignores case.
			switch strings.ToLower(r.URL.Query().Get("query")) {
			case "premier contact", "arrival":
				title := "Premier Contact"
				if r.URL.Query().Get("language") == "en-US" {
					title = "Arrival"
				}
				reply(w, map[string]any{"results": []map[string]any{{"id": 329865, "title": title, "original_title": "Arrival", "release_date": "2016-11-10", "poster_path": "/p.jpg"}}})
			case "brazil", "brazil!":
				reply(w, map[string]any{"results": []map[string]any{{"id": 68, "title": "Brazil", "original_title": "Brazil", "release_date": "1985-02-20"}}})
			default:
				reply(w, map[string]any{"results": []any{}})
			}
		case "/search/tv":
			reply(w, map[string]any{"results": []map[string]any{{"id": 95396, "name": "Severance", "original_name": "Severance", "first_air_date": "2022-02-17"}}})
		case "/movie/329865":
			reply(w, map[string]any{
				"id": 329865, "title": "Premier Contact", "original_title": "Arrival", "release_date": "2016-11-10",
				"runtime": 116, "genres": []map[string]any{{"name": "Drame"}, {"name": "Science-Fiction"}},
				"overview": "Douze vaisseaux...", "vote_average": 7.583, "vote_count": 18000,
				"poster_path": "/p.jpg", "backdrop_path": "/b.jpg",
				"credits": map[string]any{
					"cast": []map[string]any{{"name": "Amy Adams", "character": "Louise", "profile_path": "/a.jpg"}},
					"crew": []map[string]any{{"name": "Denis Villeneuve", "job": "Director"}, {"name": "Someone", "job": "Writer"}},
				},
				"videos": map[string]any{"results": []map[string]any{
					{"site": "YouTube", "type": "Teaser", "key": "teaser", "iso_639_1": "fr"},
					{"site": "YouTube", "type": "Trailer", "key": "enTrailer", "iso_639_1": "en", "official": true},
					{"site": "YouTube", "type": "Trailer", "key": "frTrailer", "iso_639_1": "fr"},
				}},
				"external_ids": map[string]any{"imdb_id": "tt2543164"},
			})
		case "/tv/95396":
			reply(w, map[string]any{
				"id": 95396, "name": "Severance", "original_name": "Severance", "first_air_date": "2022-02-17",
				"episode_run_time": []int{55}, "number_of_seasons": 2, "number_of_episodes": 19,
				"created_by": []map[string]any{{"name": "Dan Erickson"}}, "vote_average": 8.4, "vote_count": 2000,
				"external_ids": map[string]any{"imdb_id": "tt11280740", "tvdb_id": 371980},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			reply(w, map[string]any{"status_message": "The resource you requested could not be found."})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSearchAndAuth(t *testing.T) {
	server := fakeTMDB(t, nil)
	results, err := New("key").WithBaseURL(server.URL).Search(context.Background(), store.MediaKindMovie, "Arrival", "en-US")
	if err != nil || len(results) != 1 {
		t.Fatalf("search: %v %+v", err, results)
	}
	got := results[0]
	if got.Title != "Arrival" || got.Year != 2016 || got.ExternalIDs.TMDB != 329865 || got.PosterURL != "https://image.tmdb.org/t/p/w500/p.jpg" {
		t.Fatalf("result: %+v", got)
	}
	// A v4 read access token goes in the Authorization header.
	if _, err := New("eyJtoken").WithBaseURL(server.URL).Ping(context.Background()); err != nil {
		t.Fatalf("bearer token: %v", err)
	}
	_, err = New("wrong").WithBaseURL(server.URL).Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("bad key must give TMDB's message, got %v", err)
	}
}

func TestMovieDetails(t *testing.T) {
	server := fakeTMDB(t, nil)
	details, err := New("key").WithBaseURL(server.URL).Details(context.Background(), store.MediaKindMovie, 329865, "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	if details.Title != "Premier Contact" || details.OriginalTitle != "Arrival" || details.Year != 2016 || details.RuntimeMinutes != 116 {
		t.Fatalf("basics: %+v", details)
	}
	if details.TMDBRating != 7.6 || details.TMDBVotes != 18000 || details.IMDBID != "tt2543164" {
		t.Fatalf("rating/ids: %+v", details)
	}
	if len(details.Directors) != 1 || details.Directors[0] != "Denis Villeneuve" || len(details.Genres) != 2 {
		t.Fatalf("crew/genres: %+v", details)
	}
	if len(details.Cast) != 1 || details.Cast[0].PhotoURL != "https://image.tmdb.org/t/p/w185/a.jpg" {
		t.Fatalf("cast: %+v", details.Cast)
	}
	// A trailer beats a teaser, and the user's language beats "official".
	if details.TrailerURL != "https://www.youtube.com/watch?v=frTrailer" {
		t.Fatalf("trailer: %s", details.TrailerURL)
	}
	if details.TMDBURL != "https://www.themoviedb.org/movie/329865" || details.BackdropURL == "" {
		t.Fatalf("links: %+v", details)
	}
}

func TestShowDetails(t *testing.T) {
	server := fakeTMDB(t, nil)
	details, err := New("key").WithBaseURL(server.URL).Details(context.Background(), store.MediaKindShow, 95396, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if details.Seasons != 2 || details.Episodes != 19 || details.RuntimeMinutes != 55 || details.TVDBID != 371980 {
		t.Fatalf("show: %+v", details)
	}
	if len(details.Directors) != 1 || details.Directors[0] != "Dan Erickson" || details.TMDBURL != "https://www.themoviedb.org/tv/95396" {
		t.Fatalf("creators: %+v", details)
	}
	if details.Genres == nil || details.Cast == nil {
		t.Fatal("lists must be [] not null")
	}
}

func TestFindMatchSafety(t *testing.T) {
	server := fakeTMDB(t, nil)
	client := New("key").WithBaseURL(server.URL)
	ctx := context.Background()

	// French title, year known: matched.
	match, err := client.FindMatch(ctx, store.MediaKindMovie, "Premier contact", 2016)
	if err != nil || match == nil || match.TMDBID != 329865 {
		t.Fatalf("French title: %+v %v", match, err)
	}
	// Year off by more than one: refused.
	if match, _ := client.FindMatch(ctx, store.MediaKindMovie, "Brazil", 2019); match != nil {
		t.Fatalf("wrong year must not match: %+v", match)
	}
	// No year: the title alone decides.
	if match, _ := client.FindMatch(ctx, store.MediaKindMovie, "brazil!", 0); match == nil || match.TMDBID != 68 {
		t.Fatalf("no year: %+v", match)
	}
	// Different title: refused.
	if match, _ := client.FindMatch(ctx, store.MediaKindMovie, "Unknown movie", 0); match != nil {
		t.Fatalf("unknown title: %+v", match)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Amélie": "amelie",
		"  Le Fabuleux Destin d'Amélie Poulain ": "le fabuleux destin d amelie poulain",
		"Spider-Man: No Way Home":                "spider man no way home",
	}
	for input, want := range cases {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
	if Language("fr-FR,fr;q=0.9") != "fr-FR" || Language("en") != "en-US" || Language("") != "en-US" {
		t.Fatal("Language mapping")
	}
}
