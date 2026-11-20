// Package arr talks to Radarr (movies), Sonarr (TV shows) and Prowlarr
// (indexer search). Their API keys stay on the server: the app only talks to
// this API.
package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"familyapp/internal/config"
	"familyapp/internal/store"
)

// ErrUpstream means the tool did not answer or answered with an error.
var ErrUpstream = errors.New("media tool error")

// client is the HTTP part shared by the three tools (same auth, same JSON).
type client struct {
	name    string
	baseURL string
	apiKey  string
	http    *http.Client
}

func newClient(name string, cfg config.ArrConfig) *client {
	return &client{
		name:    name,
		baseURL: strings.TrimRight(cfg.URL, "/"),
		apiKey:  cfg.APIKey,
		// A Prowlarr search asks every indexer, so it can take a while.
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// do sends a request and decodes the JSON answer into out (when not nil).
func (c *client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	request.Header.Set("X-Api-Key", c.apiKey)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %s is not reachable: %v", ErrUpstream, c.name, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return fmt.Errorf("%w: read %s answer: %v", ErrUpstream, c.name, err)
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("%w: %s answered %d: %s", ErrUpstream, c.name, response.StatusCode, upstreamMessage(data))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: unexpected %s answer: %v", ErrUpstream, c.name, err)
	}
	return nil
}

// upstreamMessage extracts a short reason from an *arr error body. Radarr and
// Sonarr answer validation errors as [{"errorMessage": "..."}].
func upstreamMessage(data []byte) string {
	var validation []struct {
		ErrorMessage string `json:"errorMessage"`
	}
	if json.Unmarshal(data, &validation) == nil && len(validation) > 0 && validation[0].ErrorMessage != "" {
		return validation[0].ErrorMessage
	}
	var single struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &single) == nil && single.Message != "" {
		return single.Message
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

// --- Radarr and Sonarr ----------------------------------------------------------

// SearchResult is a movie or show found by Radarr or Sonarr, ready to be added
// to the watchlist.
type SearchResult struct {
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	Year        int               `json:"year,omitempty"`
	Overview    string            `json:"overview,omitempty"`
	PosterURL   string            `json:"posterUrl,omitempty"`
	ExternalIDs store.ExternalIDs `json:"externalIds"`
	// InLibrary is true when the tool already has this title.
	InLibrary bool `json:"inLibrary"`
}

// Library is Radarr or Sonarr: both search titles and add them for download.
type Library struct {
	client *client
	kind   string // store.MediaKindMovie or store.MediaKindShow
	cfg    config.ArrConfig
}

// lookupItem holds the fields read from a Radarr/Sonarr lookup result. The raw
// object is kept too, because adding a title means posting it back.
type lookupItem struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Year         int    `json:"year"`
	Overview     string `json:"overview"`
	RemotePoster string `json:"remotePoster"`
	TMDBID       int    `json:"tmdbId"`
	TVDBID       int    `json:"tvdbId"`
	IMDBID       string `json:"imdbId"`
	Images       []struct {
		CoverType string `json:"coverType"`
		RemoteURL string `json:"remoteUrl"`
	} `json:"images"`
}

func (l *Library) resourcePath() string {
	if l.kind == store.MediaKindMovie {
		return "/api/v3/movie"
	}
	return "/api/v3/series"
}

// Search finds titles by name.
func (l *Library) Search(ctx context.Context, term string) ([]SearchResult, error) {
	_, items, err := l.lookup(ctx, term)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		results = append(results, l.toResult(item))
	}
	return results, nil
}

// Add asks the tool to monitor and download a title, found by its TMDB id
// (movies) or TVDB id (shows). It does nothing when the title is already there.
func (l *Library) Add(ctx context.Context, ids store.ExternalIDs) (alreadyThere bool, err error) {
	var term string
	switch {
	case l.kind == store.MediaKindMovie && ids.TMDB != 0:
		term = fmt.Sprintf("tmdb:%d", ids.TMDB)
	case l.kind == store.MediaKindShow && ids.TVDB != 0:
		term = fmt.Sprintf("tvdb:%d", ids.TVDB)
	default:
		return false, ErrMissingID
	}
	raw, items, err := l.lookup(ctx, term)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, fmt.Errorf("%w: %s found no title for %s", ErrUpstream, l.client.name, term)
	}
	if items[0].ID != 0 {
		return true, nil
	}

	profileID, rootFolder, err := l.destination(ctx)
	if err != nil {
		return false, err
	}
	body := raw[0]
	body["qualityProfileId"] = profileID
	body["rootFolderPath"] = rootFolder
	body["monitored"] = true
	if l.kind == store.MediaKindMovie {
		body["minimumAvailability"] = "released"
		body["addOptions"] = map[string]any{"searchForMovie": true}
	} else {
		body["seasonFolder"] = true
		body["addOptions"] = map[string]any{"monitor": "all", "searchForMissingEpisodes": true}
	}
	return false, l.client.do(ctx, http.MethodPost, l.resourcePath(), nil, body, nil)
}

// ErrMissingID means the watchlist entry has no TMDB/TVDB id to send.
var ErrMissingID = errors.New("this title has no TMDB/TVDB id")

func (l *Library) lookup(ctx context.Context, term string) ([]map[string]any, []lookupItem, error) {
	query := url.Values{"term": {term}}
	var raw []map[string]any
	if err := l.client.do(ctx, http.MethodGet, l.resourcePath()+"/lookup", query, nil, &raw); err != nil {
		return nil, nil, err
	}
	// Decode the same JSON a second time into typed fields.
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, err
	}
	var items []lookupItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, nil, err
	}
	return raw, items, nil
}

func (l *Library) toResult(item lookupItem) SearchResult {
	poster := item.RemotePoster
	if poster == "" {
		for _, image := range item.Images {
			if image.CoverType == "poster" {
				poster = image.RemoteURL
				break
			}
		}
	}
	// The watchlist only keeps https posters.
	if !strings.HasPrefix(poster, "https://") {
		poster = ""
	}
	return SearchResult{
		Kind:        l.kind,
		Title:       item.Title,
		Year:        item.Year,
		Overview:    item.Overview,
		PosterURL:   poster,
		ExternalIDs: store.ExternalIDs{TMDB: item.TMDBID, TVDB: item.TVDBID, IMDB: item.IMDBID},
		InLibrary:   item.ID != 0,
	}
}

// destination returns the quality profile and root folder to add titles with:
// the configured ones, or the first ones of the tool.
func (l *Library) destination(ctx context.Context) (int, string, error) {
	profileID, rootFolder := l.cfg.QualityProfileID, l.cfg.RootFolder
	if profileID == 0 {
		var profiles []struct {
			ID int `json:"id"`
		}
		if err := l.client.do(ctx, http.MethodGet, "/api/v3/qualityprofile", nil, nil, &profiles); err != nil {
			return 0, "", err
		}
		if len(profiles) == 0 {
			return 0, "", fmt.Errorf("%w: %s has no quality profile", ErrUpstream, l.client.name)
		}
		profileID = profiles[0].ID
	}
	if rootFolder == "" {
		var folders []struct {
			Path string `json:"path"`
		}
		if err := l.client.do(ctx, http.MethodGet, "/api/v3/rootfolder", nil, nil, &folders); err != nil {
			return 0, "", err
		}
		if len(folders) == 0 {
			return 0, "", fmt.Errorf("%w: %s has no root folder", ErrUpstream, l.client.name)
		}
		rootFolder = folders[0].Path
	}
	return profileID, rootFolder, nil
}

// --- Prowlarr -------------------------------------------------------------------

// Release is one result of an indexer search.
type Release struct {
	GUID        string    `json:"guid"`
	IndexerID   int       `json:"indexerId"`
	Indexer     string    `json:"indexer"`
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	Seeders     *int      `json:"seeders,omitempty"`
	Leechers    *int      `json:"leechers,omitempty"`
	Protocol    string    `json:"protocol"`
	PublishDate time.Time `json:"publishDate"`
}

// Indexers is Prowlarr: it searches every indexer and sends a release to the
// download client.
type Indexers struct {
	client *client
}

// Newznab categories Prowlarr uses: 2000 movies, 5000 TV.
var categories = map[string]string{
	store.MediaKindMovie: "2000",
	store.MediaKindShow:  "5000",
}

// Search runs an interactive search. kind may be empty to search everything.
func (p *Indexers) Search(ctx context.Context, term, kind string) ([]Release, error) {
	query := url.Values{"query": {term}, "type": {"search"}, "limit": {"100"}}
	if category, ok := categories[kind]; ok {
		query.Set("categories", category)
	}
	releases := []Release{}
	if err := p.client.do(ctx, http.MethodGet, "/api/v1/search", query, nil, &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// Grab sends a release to the download client set in Prowlarr.
func (p *Indexers) Grab(ctx context.Context, guid string, indexerID int) error {
	body := map[string]any{"guid": guid, "indexerId": indexerID}
	return p.client.do(ctx, http.MethodPost, "/api/v1/search", nil, body, nil)
}

// --- All tools ------------------------------------------------------------------

// Services holds the configured tools. A nil field means "not configured".
type Services struct {
	Radarr   *Library
	Sonarr   *Library
	Prowlarr *Indexers
}

// New builds the services from the config, skipping the tools without a URL/key.
func New(radarr, sonarr, prowlarr config.ArrConfig) *Services {
	services := &Services{}
	if radarr.Enabled() {
		services.Radarr = &Library{client: newClient("Radarr", radarr), kind: store.MediaKindMovie, cfg: radarr}
	}
	if sonarr.Enabled() {
		services.Sonarr = &Library{client: newClient("Sonarr", sonarr), kind: store.MediaKindShow, cfg: sonarr}
	}
	if prowlarr.Enabled() {
		services.Prowlarr = &Indexers{client: newClient("Prowlarr", prowlarr)}
	}
	return services
}

// Library returns Radarr for movies and Sonarr for shows, or nil.
func (s *Services) Library(kind string) *Library {
	switch kind {
	case store.MediaKindMovie:
		return s.Radarr
	case store.MediaKindShow:
		return s.Sonarr
	}
	return nil
}
