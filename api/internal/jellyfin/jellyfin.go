// Package jellyfin reads a Jellyfin server: its users, the movies and shows of
// a user, and what that user played. It only reads; it never changes Jellyfin.
package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUpstream wraps every error that comes from Jellyfin (down, bad key...).
var ErrUpstream = errors.New("Jellyfin error")

// PageSize is how many items one /Items call returns (tests make it small).
var PageSize = 500

// Kinds of Item, the same words as the watchlist.
const (
	KindMovie = "movie"
	KindShow  = "show"
)

// Client calls one Jellyfin server with an API key.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// New returns a client for the server at baseURL (for example
// http://host.docker.internal:8096) with an API key from the Jellyfin
// dashboard (Dashboard > API Keys).
func New(baseURL, key string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		key:     strings.TrimSpace(key),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// User is a Jellyfin account.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Item is a movie or a show of the library, with the played state of one user.
type Item struct {
	ID    string
	Kind  string
	Title string
	Year  int
	TMDB  int
	TVDB  int
	// Played is true for a watched movie, or a show with every episode watched.
	Played bool
	// Partly is true for a show with some episodes watched, not all.
	Partly bool
	// PlayedAt is the last time the movie (or an episode of the show) was
	// played. Nil when Jellyfin does not know.
	PlayedAt *time.Time
}

// Ping checks the address and the key. It returns "Jellyfin <version>".
func (c *Client) Ping(ctx context.Context) (string, error) {
	var answer struct {
		Version string `json:"Version"`
	}
	if err := c.get(ctx, "/System/Info", nil, &answer); err != nil {
		return "", err
	}
	return strings.TrimSpace("Jellyfin " + answer.Version), nil
}

// Users lists the Jellyfin accounts.
func (c *Client) Users(ctx context.Context) ([]User, error) {
	var answer []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := c.get(ctx, "/Users", nil, &answer); err != nil {
		return nil, err
	}
	users := make([]User, 0, len(answer))
	for _, user := range answer {
		users = append(users, User{ID: user.ID, Name: user.Name})
	}
	return users, nil
}

// rawItem is the part of a Jellyfin item we read.
type rawItem struct {
	ID             string            `json:"Id"`
	Name           string            `json:"Name"`
	Type           string            `json:"Type"`
	ProductionYear int               `json:"ProductionYear"`
	ProviderIDs    map[string]string `json:"ProviderIds"`
	SeriesID       string            `json:"SeriesId"`
	UserData       struct {
		Played           bool       `json:"Played"`
		PlayedPercentage float64    `json:"PlayedPercentage"`
		LastPlayedDate   *time.Time `json:"LastPlayedDate"`
	} `json:"UserData"`
}

type itemsPage struct {
	Items            []rawItem `json:"Items"`
	TotalRecordCount int       `json:"TotalRecordCount"`
}

// Library returns the movies and shows a user can see, with their played
// state. Items without a TMDB and a TVDB id (home videos, YouTube downloads...)
// are left out: they can never match a watchlist title.
func (c *Client) Library(ctx context.Context, userID string) ([]Item, error) {
	if userID == "" {
		return nil, fmt.Errorf("%w: no Jellyfin user", ErrUpstream)
	}
	raw, err := c.items(ctx, userID, url.Values{
		"IncludeItemTypes": {"Movie,Series"},
		"Fields":           {"ProviderIds,ProductionYear"},
	})
	if err != nil {
		return nil, err
	}
	// Shows have no "last played" date of their own: it is the date of their
	// last played episode.
	episodes, err := c.items(ctx, userID, url.Values{
		"IncludeItemTypes": {"Episode"},
		"Filters":          {"IsPlayed"},
	})
	if err != nil {
		return nil, err
	}
	showPlayedAt := make(map[string]time.Time)
	for _, episode := range episodes {
		date := episode.UserData.LastPlayedDate
		if episode.SeriesID == "" || date == nil {
			continue
		}
		if latest, ok := showPlayedAt[episode.SeriesID]; !ok || date.After(latest) {
			showPlayedAt[episode.SeriesID] = *date
		}
	}

	items := make([]Item, 0, len(raw))
	for _, entry := range raw {
		item := Item{
			ID:     entry.ID,
			Title:  strings.TrimSpace(entry.Name),
			Year:   entry.ProductionYear,
			TMDB:   providerID(entry.ProviderIDs, "Tmdb"),
			TVDB:   providerID(entry.ProviderIDs, "Tvdb"),
			Played: entry.UserData.Played,
		}
		if item.TMDB == 0 && item.TVDB == 0 {
			continue
		}
		switch entry.Type {
		case "Movie":
			item.Kind = KindMovie
			item.PlayedAt = utc(entry.UserData.LastPlayedDate)
		case "Series":
			item.Kind = KindShow
			date, watchedEpisode := showPlayedAt[entry.ID]
			if watchedEpisode {
				item.PlayedAt = utc(&date)
			}
			item.Partly = !item.Played && (entry.UserData.PlayedPercentage > 0 || watchedEpisode)
		default:
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// items reads every page of /Items for a user.
func (c *Client) items(ctx context.Context, userID string, filter url.Values) ([]rawItem, error) {
	all := []rawItem{}
	for start := 0; ; start += PageSize {
		query := url.Values{
			"userId":         {userID},
			"Recursive":      {"true"},
			"StartIndex":     {strconv.Itoa(start)},
			"Limit":          {strconv.Itoa(PageSize)},
			"enableImages":   {"false"},
			"EnableUserData": {"true"},
		}
		for key, values := range filter {
			query[key] = values
		}
		var page itemsPage
		if err := c.get(ctx, "/Items", query, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) < PageSize || len(all) >= page.TotalRecordCount {
			return all, nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	address := c.baseURL + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fmt.Errorf("%w: bad address: %v", ErrUpstream, err)
	}
	request.Header.Set("Authorization", `MediaBrowser Token="`+c.key+`"`)
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: Jellyfin is not reachable", ErrUpstream)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 50<<20))
	if err != nil {
		return fmt.Errorf("%w: read answer: %v", ErrUpstream, err)
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: Jellyfin answered %d: the API key is not valid", ErrUpstream, response.StatusCode)
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: Jellyfin answered %d", ErrUpstream, response.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: unexpected answer: %v", ErrUpstream, err)
	}
	return nil
}

// providerID reads a numeric provider id ("Tmdb": "329865"); 0 when missing.
func providerID(ids map[string]string, name string) int {
	value, err := strconv.Atoi(strings.TrimSpace(ids[name]))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// utc returns a copy of a date in UTC, rounded to milliseconds like MongoDB.
func utc(date *time.Time) *time.Time {
	if date == nil || date.IsZero() || date.Year() < 2000 {
		return nil
	}
	value := date.UTC().Truncate(time.Millisecond)
	return &value
}
