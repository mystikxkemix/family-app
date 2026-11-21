// Package tmdb reads movie and show data from The Movie Database (TMDB):
// search, details, ids. TMDB also gives the IMDb and TVDB ids, so no other
// metadata source is needed. The key stays on the server.
//
// This product uses the TMDB API but is not endorsed or certified by TMDB.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"familyapp/internal/arr"
	"familyapp/internal/store"
)

// ErrUpstream means TMDB did not answer or answered with an error.
var ErrUpstream = errors.New("TMDB error")

const (
	defaultBaseURL = "https://api.themoviedb.org/3"
	imageBase      = "https://image.tmdb.org/t/p/"
	// maxCast is how many actors the details page shows.
	maxCast = 8
)

// Client calls the TMDB API v3.
type Client struct {
	key     string
	baseURL string
	http    *http.Client
}

// New returns a client. The key is either a v3 "API key" or a v4 "read access
// token" (a JWT, starting with "eyJ"); both work with the v3 API.
func New(key string) *Client {
	return &Client{key: strings.TrimSpace(key), baseURL: defaultBaseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// WithBaseURL points the client to another server (tests).
func (c *Client) WithBaseURL(base string) *Client {
	c.baseURL = strings.TrimRight(base, "/")
	return c
}

// Language turns "fr", "fr-FR" or an Accept-Language value into the TMDB
// language: French for French, English otherwise.
func Language(value string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "fr") {
		return "fr-FR"
	}
	return "en-US"
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		return err
	}
	if strings.HasPrefix(c.key, "eyJ") {
		request.Header.Set("Authorization", "Bearer "+c.key)
	} else {
		query.Set("api_key", c.key)
	}
	request.URL, err = url.Parse(c.baseURL + path + "?" + query.Encode())
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: TMDB is not reachable", ErrUpstream)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("%w: read answer: %v", ErrUpstream, err)
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			StatusMessage string `json:"status_message"`
		}
		_ = json.Unmarshal(data, &failure)
		if failure.StatusMessage == "" {
			failure.StatusMessage = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("%w: TMDB answered %d: %s", ErrUpstream, response.StatusCode, failure.StatusMessage)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: unexpected answer: %v", ErrUpstream, err)
	}
	return nil
}

// Ping checks the key.
func (c *Client) Ping(ctx context.Context) (string, error) {
	var answer struct {
		Images struct {
			SecureBaseURL string `json:"secure_base_url"`
		} `json:"images"`
	}
	if err := c.get(ctx, "/configuration", nil, &answer); err != nil {
		return "", err
	}
	return "TMDB", nil
}

// --- Search -----------------------------------------------------------------------

type searchHit struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	Overview      string `json:"overview"`
	PosterPath    string `json:"poster_path"`
}

func (h searchHit) title() string {
	if h.Title != "" {
		return h.Title
	}
	return h.Name
}

func (h searchHit) originalTitle() string {
	if h.OriginalTitle != "" {
		return h.OriginalTitle
	}
	return h.OriginalName
}

func (h searchHit) year() int {
	date := h.ReleaseDate
	if date == "" {
		date = h.FirstAirDate
	}
	return yearOf(date)
}

func (c *Client) search(ctx context.Context, kind, query, language string) ([]searchHit, error) {
	path := "/search/movie"
	if kind == store.MediaKindShow {
		path = "/search/tv"
	}
	var answer struct {
		Results []searchHit `json:"results"`
	}
	params := url.Values{"query": {query}, "language": {language}, "include_adult": {"false"}}
	if err := c.get(ctx, path, params, &answer); err != nil {
		return nil, err
	}
	return answer.Results, nil
}

// Search finds titles by name, in the given language. TMDB matches the query
// against every translation, so a French or an English title both work.
func (c *Client) Search(ctx context.Context, kind, query, language string) ([]arr.SearchResult, error) {
	hits, err := c.search(ctx, kind, query, language)
	if err != nil {
		return nil, err
	}
	results := make([]arr.SearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, arr.SearchResult{
			Kind:        kind,
			Title:       hit.title(),
			Year:        hit.year(),
			Overview:    hit.Overview,
			PosterURL:   image("w500", hit.PosterPath),
			ExternalIDs: store.ExternalIDs{TMDB: hit.ID},
		})
	}
	return results, nil
}

// --- Details ----------------------------------------------------------------------

// CastMember is one actor of the details page.
type CastMember struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	PhotoURL  string `json:"photoUrl,omitempty"`
}

// Details is everything the details page shows.
type Details struct {
	TMDBID         int          `json:"tmdbId"`
	TVDBID         int          `json:"tvdbId,omitempty"`
	IMDBID         string       `json:"imdbId,omitempty"`
	Kind           string       `json:"kind"`
	Title          string       `json:"title"`
	OriginalTitle  string       `json:"originalTitle"`
	Year           int          `json:"year,omitempty"`
	RuntimeMinutes int          `json:"runtimeMinutes,omitempty"`
	Seasons        int          `json:"seasons,omitempty"`
	Episodes       int          `json:"episodes,omitempty"`
	Genres         []string     `json:"genres"`
	Overview       string       `json:"overview"`
	TMDBRating     float64      `json:"tmdbRating"`
	TMDBVotes      int          `json:"tmdbVotes"`
	Directors      []string     `json:"directors"`
	Cast           []CastMember `json:"cast"`
	TrailerURL     string       `json:"trailerUrl,omitempty"`
	PosterURL      string       `json:"posterUrl,omitempty"`
	BackdropURL    string       `json:"backdropUrl,omitempty"`
	TMDBURL        string       `json:"tmdbUrl"`
}

// rawDetails holds the fields of /movie/{id} and /tv/{id} we read.
type rawDetails struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	ReleaseDate      string `json:"release_date"`
	FirstAirDate     string `json:"first_air_date"`
	Runtime          int    `json:"runtime"`
	EpisodeRunTime   []int  `json:"episode_run_time"`
	NumberOfSeasons  int    `json:"number_of_seasons"`
	NumberOfEpisodes int    `json:"number_of_episodes"`
	Genres           []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Overview     string  `json:"overview"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	CreatedBy    []struct {
		Name string `json:"name"`
	} `json:"created_by"`
	Credits struct {
		Cast []struct {
			Name        string `json:"name"`
			Character   string `json:"character"`
			ProfilePath string `json:"profile_path"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	Videos struct {
		Results []struct {
			Site     string `json:"site"`
			Type     string `json:"type"`
			Key      string `json:"key"`
			Official bool   `json:"official"`
			Language string `json:"iso_639_1"`
		} `json:"results"`
	} `json:"videos"`
	ExternalIDs struct {
		IMDBID string `json:"imdb_id"`
		TVDBID int    `json:"tvdb_id"`
	} `json:"external_ids"`
}

// Details reads one movie or show, with credits, videos and external ids.
func (c *Client) Details(ctx context.Context, kind string, id int, language string) (*Details, error) {
	path := "/movie/" + strconv.Itoa(id)
	if kind == store.MediaKindShow {
		path = "/tv/" + strconv.Itoa(id)
	}
	short := strings.SplitN(language, "-", 2)[0]
	params := url.Values{
		"language":               {language},
		"append_to_response":     {"credits,videos,external_ids"},
		"include_video_language": {short + ",en,null"},
	}
	var raw rawDetails
	if err := c.get(ctx, path, params, &raw); err != nil {
		return nil, err
	}
	return raw.toDetails(kind, short), nil
}

func (raw rawDetails) toDetails(kind, language string) *Details {
	details := &Details{
		TMDBID:      raw.ID,
		TVDBID:      raw.ExternalIDs.TVDBID,
		IMDBID:      raw.ExternalIDs.IMDBID,
		Kind:        kind,
		Overview:    raw.Overview,
		TMDBRating:  math.Round(raw.VoteAverage*10) / 10,
		TMDBVotes:   raw.VoteCount,
		Genres:      []string{},
		Directors:   []string{},
		Cast:        []CastMember{},
		PosterURL:   image("w500", raw.PosterPath),
		BackdropURL: image("w1280", raw.BackdropPath),
	}
	for _, genre := range raw.Genres {
		details.Genres = append(details.Genres, genre.Name)
	}
	if kind == store.MediaKindShow {
		details.Title, details.OriginalTitle = raw.Name, raw.OriginalName
		details.Year = yearOf(raw.FirstAirDate)
		details.Seasons, details.Episodes = raw.NumberOfSeasons, raw.NumberOfEpisodes
		if len(raw.EpisodeRunTime) > 0 {
			details.RuntimeMinutes = raw.EpisodeRunTime[0]
		}
		for _, creator := range raw.CreatedBy {
			details.Directors = append(details.Directors, creator.Name)
		}
		details.TMDBURL = fmt.Sprintf("https://www.themoviedb.org/tv/%d", raw.ID)
	} else {
		details.Title, details.OriginalTitle = raw.Title, raw.OriginalTitle
		details.Year = yearOf(raw.ReleaseDate)
		details.RuntimeMinutes = raw.Runtime
		for _, member := range raw.Credits.Crew {
			if member.Job == "Director" {
				details.Directors = append(details.Directors, member.Name)
			}
		}
		details.TMDBURL = fmt.Sprintf("https://www.themoviedb.org/movie/%d", raw.ID)
	}
	for i, actor := range raw.Credits.Cast {
		if i == maxCast {
			break
		}
		details.Cast = append(details.Cast, CastMember{Name: actor.Name, Character: actor.Character, PhotoURL: image("w185", actor.ProfilePath)})
	}
	details.TrailerURL = trailer(raw, language)
	return details
}

// trailer picks a YouTube trailer: the user's language first, official first.
func trailer(raw rawDetails, language string) string {
	best, bestScore := "", -1
	for _, video := range raw.Videos.Results {
		if video.Site != "YouTube" || (video.Type != "Trailer" && video.Type != "Teaser") || video.Key == "" {
			continue
		}
		score := 0
		if video.Type == "Trailer" {
			score += 4
		}
		if video.Language == language {
			score += 2
		}
		if video.Official {
			score++
		}
		if score > bestScore {
			best, bestScore = "https://www.youtube.com/watch?v="+url.QueryEscape(video.Key), score
		}
	}
	return best
}

// --- Matching ---------------------------------------------------------------------

// Match is a safe automatic match for a title.
type Match struct {
	TMDBID int
	Year   int
}

// FindMatch returns the TMDB title for a watchlist entry, or nil when no
// result is safe enough: the first result must have the same normalized title
// (in French or English, or the original title), and the same year ±1 when
// the year is known. A wrong movie with the same name is worse than none.
func (c *Client) FindMatch(ctx context.Context, kind, title string, year int) (*Match, error) {
	wanted := Normalize(title)
	if wanted == "" {
		return nil, nil
	}
	for _, language := range []string{"fr-FR", "en-US"} {
		hits, err := c.search(ctx, kind, title, language)
		if err != nil {
			return nil, err
		}
		if len(hits) == 0 {
			return nil, nil
		}
		first := hits[0]
		sameTitle := Normalize(first.title()) == wanted || Normalize(first.originalTitle()) == wanted
		sameYear := year == 0 || first.year() == 0 || abs(first.year()-year) <= 1
		if sameTitle && sameYear {
			return &Match{TMDBID: first.ID, Year: first.year()}, nil
		}
	}
	return nil, nil
}

// Normalize makes titles comparable: lower case, no accents, no punctuation,
// single spaces.
func Normalize(title string) string {
	var builder strings.Builder
	space := false
	for _, r := range strings.ToLower(title) {
		r = unaccent(r)
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			space = false
		case r > 127:
			// Other scripts (Japanese...) are kept as they are.
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			space = false
		default:
			space = true
		}
	}
	return builder.String()
}

// unaccent maps the accented Latin letters of French and close languages.
func unaccent(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ä', 'ã', 'å':
		return 'a'
	case 'ç':
		return 'c'
	case 'è', 'é', 'ê', 'ë':
		return 'e'
	case 'ì', 'í', 'î', 'ï':
		return 'i'
	case 'ñ':
		return 'n'
	case 'ò', 'ó', 'ô', 'ö', 'õ':
		return 'o'
	case 'ù', 'ú', 'û', 'ü':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	case 'œ':
		return 'o'
	case 'æ':
		return 'a'
	}
	return r
}

func image(size, path string) string {
	if path == "" {
		return ""
	}
	return imageBase + size + path
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return year
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
