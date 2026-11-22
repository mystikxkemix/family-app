package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"familyapp/internal/store"
	"familyapp/internal/tmdb"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var errNotMatched = &apiError{Status: http.StatusNotFound, Code: "not_matched", Message: "this title is not linked to TMDB yet"}

// tmdbClient returns the TMDB client in use, or nil when no key is set.
func (s *Server) tmdbClient() *tmdb.Client {
	return s.tmdb.Load()
}

// tmdbError maps TMDB errors to API errors.
func tmdbError(err error) error {
	if errors.Is(err, tmdb.ErrUpstream) {
		return &apiError{Status: http.StatusBadGateway, Code: "upstream", Message: err.Error()}
	}
	return err
}

// requestLanguage is the TMDB language of a request: the "lang" query
// parameter, else the Accept-Language header.
func requestLanguage(r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return tmdb.Language(lang)
	}
	return tmdb.Language(r.Header.Get("Accept-Language"))
}

// mediaDetails returns the TMDB details of a watchlist title, from the cache
// when possible.
func (s *Server) mediaDetails(w http.ResponseWriter, r *http.Request, caller principal) error {
	client := s.tmdbClient()
	if client == nil {
		return notConfigured("TMDB")
	}
	mediaID, err := pathID(r, "mediaId")
	if err != nil {
		return err
	}
	media, err := s.store.MediaByID(r.Context(), caller.HouseholdID, mediaID)
	if err != nil {
		return err
	}
	if media.ExternalIDs.TMDB == 0 {
		return errNotMatched
	}
	details, err := s.cachedDetails(r.Context(), client, media.Kind, media.ExternalIDs.TMDB, requestLanguage(r))
	if err != nil {
		return tmdbError(err)
	}
	// Fill the ids Sonarr and Radarr need, the first time we learn them.
	if (media.ExternalIDs.TVDB == 0 && details.TVDBID != 0) || (media.ExternalIDs.IMDB == "" && details.IMDBID != "") {
		ids := bson.M{"externalIds.tvdb": details.TVDBID, "externalIds.imdb": details.IMDBID}
		if _, err := s.store.UpdateMedia(r.Context(), caller.HouseholdID, mediaID, bson.M{"$set": ids}); err != nil {
			s.log.Warn("save external ids", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, details)
	return nil
}

// cachedDetails reads the details cache, or TMDB (and fills the cache).
func (s *Server) cachedDetails(ctx context.Context, client *tmdb.Client, kind string, tmdbID int, language string) (*tmdb.Details, error) {
	if payload, err := s.store.CachedDetails(ctx, kind, tmdbID, language); err == nil {
		var details tmdb.Details
		if json.Unmarshal(payload, &details) == nil {
			return &details, nil
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	details, err := client.Details(ctx, kind, tmdbID, language)
	if err != nil {
		return nil, err
	}
	if payload, err := json.Marshal(details); err == nil {
		if err := s.store.SaveDetails(ctx, kind, tmdbID, language, payload); err != nil {
			s.log.Warn("cache TMDB details", "error", err)
		}
	}
	return details, nil
}

type matchRequest struct {
	TMDBID int `json:"tmdbId"`
}

// matchMedia links a watchlist title to a TMDB title chosen by the user
// ("Find the details" or "Change match").
func (s *Server) matchMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	client := s.tmdbClient()
	if client == nil {
		return notConfigured("TMDB")
	}
	mediaID, err := pathID(r, "mediaId")
	if err != nil {
		return err
	}
	var request matchRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if request.TMDBID <= 0 {
		return badRequest("tmdbId is required")
	}
	media, err := s.store.MediaByID(r.Context(), caller.HouseholdID, mediaID)
	if err != nil {
		return err
	}
	details, err := s.cachedDetails(r.Context(), client, media.Kind, request.TMDBID, requestLanguage(r))
	if err != nil {
		return tmdbError(err)
	}
	updated, err := s.store.UpdateMedia(r.Context(), caller.HouseholdID, mediaID, bson.M{"$set": matchFields(details.TMDBID, details.TVDBID, details.IMDBID, details.Year, details.PosterURL)})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, updated)
	return nil
}

// matchFields is the update that links a title to TMDB. The title the user
// typed is kept; the ids, year and poster come from TMDB.
func matchFields(tmdbID, tvdbID int, imdbID string, year int, poster string) bson.M {
	set := bson.M{
		"externalIds.tmdb": tmdbID,
		"externalIds.tvdb": tvdbID,
		"externalIds.imdb": imdbID,
		"matchCheckedAt":   store.Now(),
	}
	if year > 0 {
		set["year"] = year
	}
	if poster != "" {
		set["posterUrl"] = poster
	}
	return set
}

// --- Automatic matching -------------------------------------------------------------

// startMatcher runs the automatic TMDB matching in the background: at start,
// after a title is added without a TMDB id, and after the TMDB key is saved.
func (s *Server) startMatcher() {
	s.matchTrigger = make(chan struct{}, 1)
	s.matchStop = make(chan struct{})
	s.matchDone = make(chan struct{})
	go func() {
		defer close(s.matchDone)
		for {
			select {
			case <-s.matchStop:
				return
			case <-s.matchTrigger:
				s.matchPending()
			}
		}
	}()
	s.triggerMatch()
}

// triggerMatch asks for a matching run (it never blocks).
func (s *Server) triggerMatch() {
	if s.matchTrigger == nil {
		return
	}
	select {
	case s.matchTrigger <- struct{}{}:
	default: // a run is already pending
	}
}

// Close stops the background work (TMDB matching, Jellyfin sync).
func (s *Server) Close() {
	if s.matchStop != nil {
		close(s.matchStop)
		<-s.matchDone
	}
	if s.jellyfinStop != nil {
		close(s.jellyfinStop)
		<-s.jellyfinDone
	}
}

// matchDelay spaces TMDB calls to stay far from its rate limit.
var matchDelay = 250 * time.Millisecond

// matchPending matches every title that was never checked, in batches.
func (s *Server) matchPending() {
	client := s.tmdbClient()
	if client == nil {
		return
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		pending, err := s.store.MediaToMatch(ctx, 50)
		cancel()
		if err != nil {
			s.log.Warn("automatic matching: load titles", "error", err)
			return
		}
		if len(pending) == 0 {
			return
		}
		for _, media := range pending {
			select {
			case <-s.matchStop:
				return
			default:
			}
			if err := s.matchOne(client, media); err != nil {
				// TMDB down or key wrong: stop and try again at the next trigger.
				s.log.Warn("automatic matching stopped", "error", err)
				return
			}
			time.Sleep(matchDelay)
		}
	}
}

// matchOne links one title when TMDB has a safe match (see tmdb.FindMatch),
// and marks it as checked either way.
func (s *Server) matchOne(client *tmdb.Client, media store.Media) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	match, err := client.FindMatch(ctx, media.Kind, media.Title, media.Year)
	if err != nil {
		return err
	}
	set := bson.M{}
	if match != nil {
		details, err := s.cachedDetails(ctx, client, media.Kind, match.TMDBID, "en-US")
		if err != nil {
			return err
		}
		set = matchFields(details.TMDBID, details.TVDBID, details.IMDBID, details.Year, details.PosterURL)
		if media.PosterURL != "" {
			delete(set, "posterUrl")
		}
		s.log.Info("title linked to TMDB", "title", media.Title, "tmdbId", details.TMDBID)
	}
	return s.store.SetMediaMatch(ctx, media.ID, set)
}
