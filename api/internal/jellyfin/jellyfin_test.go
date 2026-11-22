package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeServer answers like Jellyfin for a small library, in pages.
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	movies := []string{
		`{"Id":"m1","Name":"Arrival","Type":"Movie","ProductionYear":2016,"ProviderIds":{"Tmdb":"329865","Imdb":"tt2543164"},"UserData":{"Played":true,"LastPlayedDate":"2025-03-22T20:00:34.1894693Z"}}`,
		`{"Id":"m2","Name":"Home video","Type":"Movie","ProviderIds":{},"UserData":{"Played":true}}`,
		`{"Id":"s1","Name":"Dark","Type":"Series","ProductionYear":2017,"ProviderIds":{"Tmdb":"70523","Tvdb":"334824"},"UserData":{"Played":true,"PlayedPercentage":100}}`,
		`{"Id":"s2","Name":"Severance","Type":"Series","ProductionYear":2022,"ProviderIds":{"Tvdb":"371980"},"UserData":{"Played":false,"PlayedPercentage":50}}`,
		`{"Id":"s3","Name":"Not started","Type":"Series","ProviderIds":{"Tvdb":"1"},"UserData":{"Played":false}}`,
	}
	episodes := []string{
		`{"Id":"e1","Type":"Episode","SeriesId":"s1","UserData":{"Played":true,"LastPlayedDate":"2024-01-01T10:00:00Z"}}`,
		`{"Id":"e2","Type":"Episode","SeriesId":"s1","UserData":{"Played":true,"LastPlayedDate":"2024-02-01T10:00:00Z"}}`,
		`{"Id":"e3","Type":"Episode","SeriesId":"s2","UserData":{"Played":true,"LastPlayedDate":"2024-03-01T10:00:00Z"}}`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != `MediaBrowser Token="key"` {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"Version":"12.4.0","ProductName":"Jellyfin Server"}`))
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"Home"}]`))
		case "/Items":
			query := r.URL.Query()
			if query.Get("userId") != "u1" || query.Get("Recursive") != "true" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			all := movies
			if query.Get("IncludeItemTypes") == "Episode" {
				all = episodes
			}
			start, _ := strconv.Atoi(query.Get("StartIndex"))
			limit, _ := strconv.Atoi(query.Get("Limit"))
			end := min(start+limit, len(all))
			page := []string{}
			if start < len(all) {
				page = all[start:end]
			}
			_, _ = w.Write([]byte(`{"Items":[` + strings.Join(page, ",") + `],"TotalRecordCount":` + strconv.Itoa(len(all)) + `,"StartIndex":` + strconv.Itoa(start) + `}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestLibrary(t *testing.T) {
	server := fakeServer(t)
	defer server.Close()
	PageSize = 2 // three pages of movies and shows
	defer func() { PageSize = 500 }()
	client := New(server.URL+"/", " key ")
	ctx := context.Background()

	version, err := client.Ping(ctx)
	if err != nil || version != "Jellyfin 12.4.0" {
		t.Fatalf("ping: %q %v", version, err)
	}
	users, err := client.Users(ctx)
	if err != nil || len(users) != 1 || users[0].ID != "u1" || users[0].Name != "Home" {
		t.Fatalf("users: %+v %v", users, err)
	}

	items, err := client.Library(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if len(items) != 4 {
		t.Fatalf("the home video (no ids) must be skipped: %+v", items)
	}
	arrival := byID["m1"]
	if arrival.Kind != KindMovie || arrival.TMDB != 329865 || arrival.Year != 2016 || !arrival.Played || arrival.PlayedAt == nil || arrival.PlayedAt.Year() != 2025 {
		t.Fatalf("movie: %+v", arrival)
	}
	dark := byID["s1"]
	wantDate := time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)
	if dark.Kind != KindShow || !dark.Played || dark.Partly || dark.TVDB != 334824 || dark.PlayedAt == nil || !dark.PlayedAt.Equal(wantDate) {
		t.Fatalf("a played show takes the date of its last episode: %+v", dark)
	}
	if severance := byID["s2"]; severance.Played || !severance.Partly || severance.TMDB != 0 {
		t.Fatalf("partly played show: %+v", severance)
	}
	if notStarted := byID["s3"]; notStarted.Played || notStarted.Partly || notStarted.PlayedAt != nil {
		t.Fatalf("show not started: %+v", notStarted)
	}

	// A wrong key is a clear upstream error.
	_, err = New(server.URL, "wrong").Ping(ctx)
	if !errors.Is(err, ErrUpstream) || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("wrong key: %v", err)
	}
	_, err = New("http://127.0.0.1:1", "key").Users(ctx)
	if !errors.Is(err, ErrUpstream) || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("server down: %v", err)
	}
}
