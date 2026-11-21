package api

import (
	"errors"
	"net/http"
	"strings"

	"familyapp/internal/arr"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// integrations tells the app which media tools the server can use.
type integrations struct {
	Radarr   bool `json:"radarr"`
	Sonarr   bool `json:"sonarr"`
	Prowlarr bool `json:"prowlarr"`
	// TMDB gives search results and the details pages.
	TMDB bool `json:"tmdb"`
}

func (s *Server) integrations() integrations {
	services := s.arrServices()
	return integrations{
		Radarr:   services.Radarr != nil,
		Sonarr:   services.Sonarr != nil,
		Prowlarr: services.Prowlarr != nil,
		TMDB:     s.tmdbClient() != nil,
	}
}

func notConfigured(tool string) *apiError {
	return &apiError{Status: http.StatusServiceUnavailable, Code: "not_configured", Message: tool + " is not configured on the server"}
}

// arrError maps media tool errors to API errors.
func arrError(err error) error {
	switch {
	case errors.Is(err, arr.ErrMissingID):
		return &apiError{Status: http.StatusBadRequest, Code: "missing_id", Message: err.Error()}
	case errors.Is(err, arr.ErrUpstream):
		return &apiError{Status: http.StatusBadGateway, Code: "upstream", Message: err.Error()}
	}
	return err
}

// queryTerm reads and checks the "q" query parameter.
func queryTerm(r *http.Request) (string, error) {
	term, err := cleanText("q", r.URL.Query().Get("q"), true, 200)
	if err != nil {
		return "", err
	}
	return term, nil
}

// searchMedia finds movies (Radarr) or shows (Sonarr) by title, to add them
// to the watchlist with their poster and ids.
func (s *Server) searchMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	kind := r.URL.Query().Get("kind")
	if err := oneOf("kind", kind, store.MediaKindMovie, store.MediaKindShow); err != nil {
		return err
	}
	term, err := queryTerm(r)
	if err != nil {
		return err
	}
	// TMDB first: it has every title, in the user's language. Radarr/Sonarr
	// otherwise, as before.
	if client := s.tmdbClient(); client != nil {
		results, err := client.Search(r.Context(), kind, term, requestLanguage(r))
		if err != nil {
			return tmdbError(err)
		}
		writeJSON(w, http.StatusOK, results)
		return nil
	}
	library := s.arrServices().Library(kind)
	if library == nil {
		return notConfigured(map[string]string{store.MediaKindMovie: "Radarr or TMDB", store.MediaKindShow: "Sonarr or TMDB"}[kind])
	}
	results, err := library.Search(r.Context(), term)
	if err != nil {
		return arrError(err)
	}
	writeJSON(w, http.StatusOK, results)
	return nil
}

// downloadMedia sends a watchlist entry to Radarr or Sonarr.
func (s *Server) downloadMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	mediaID, err := pathID(r, "mediaId")
	if err != nil {
		return err
	}
	media, err := s.store.MediaByID(r.Context(), caller.HouseholdID, mediaID)
	if err != nil {
		return err
	}
	library := s.arrServices().Library(media.Kind)
	if library == nil {
		return notConfigured(map[string]string{store.MediaKindMovie: "Radarr", store.MediaKindShow: "Sonarr"}[media.Kind])
	}
	alreadyThere, err := library.Add(r.Context(), media.ExternalIDs)
	if err != nil {
		return arrError(err)
	}
	media, err = s.store.UpdateMedia(r.Context(), caller.HouseholdID, mediaID,
		bson.M{"$set": bson.M{"downloadRequestedAt": store.Now()}})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"media": media, "alreadyInLibrary": alreadyThere})
	return nil
}

// searchReleases runs a Prowlarr interactive search.
func (s *Server) searchReleases(w http.ResponseWriter, r *http.Request, caller principal) error {
	prowlarr := s.arrServices().Prowlarr
	if prowlarr == nil {
		return notConfigured("Prowlarr")
	}
	term, err := queryTerm(r)
	if err != nil {
		return err
	}
	kind := r.URL.Query().Get("kind")
	if kind != "" {
		if err := oneOf("kind", kind, store.MediaKindMovie, store.MediaKindShow); err != nil {
			return err
		}
	}
	releases, err := prowlarr.Search(r.Context(), term, kind)
	if err != nil {
		return arrError(err)
	}
	writeJSON(w, http.StatusOK, releases)
	return nil
}

type grabRequest struct {
	GUID      string `json:"guid"`
	IndexerID int    `json:"indexerId"`
}

// grabRelease sends a release found by searchReleases to the download client.
func (s *Server) grabRelease(w http.ResponseWriter, r *http.Request, caller principal) error {
	prowlarr := s.arrServices().Prowlarr
	if prowlarr == nil {
		return notConfigured("Prowlarr")
	}
	var request grabRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if strings.TrimSpace(request.GUID) == "" || len(request.GUID) > 2000 || request.IndexerID <= 0 {
		return badRequest("guid and indexerId are required")
	}
	if err := prowlarr.Grab(r.Context(), request.GUID, request.IndexerID); err != nil {
		return arrError(err)
	}
	s.log.Info("release grabbed", "user", caller.User.Username, "indexerId", request.IndexerID)
	noContent(w)
	return nil
}
