package api

import (
	"fmt"
	"net/http"
	"net/url"

	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type mediaRequest struct {
	Kind        *string            `json:"kind"`
	Title       *string            `json:"title"`
	Year        *int               `json:"year"`
	PosterURL   *string            `json:"posterUrl"`
	Overview    *string            `json:"overview"`
	Note        *string            `json:"note"`
	Status      *string            `json:"status"`
	ExternalIDs *store.ExternalIDs `json:"externalIds"`
	// Interested adds (true) or removes (false) the caller from the people who
	// want to watch it.
	Interested *bool `json:"interested"`
}

// fields validates the request and returns the fields it sets.
func (request mediaRequest) fields(creating bool) (bson.M, error) {
	set := bson.M{}
	var err error
	if request.Kind != nil || creating {
		if err := oneOf("kind", deref(request.Kind), store.MediaKindMovie, store.MediaKindShow); err != nil {
			return nil, err
		}
		set["kind"] = *request.Kind
	}
	if request.Title != nil || creating {
		if set["title"], err = cleanText("title", deref(request.Title), true, 200); err != nil {
			return nil, err
		}
	}
	if request.Year != nil {
		if *request.Year != 0 && (*request.Year < 1870 || *request.Year > 2200) {
			return nil, badRequest("year is not valid")
		}
		set["year"] = *request.Year
	}
	if request.PosterURL != nil {
		poster, err := cleanText("posterUrl", *request.PosterURL, false, 1000)
		if err != nil {
			return nil, err
		}
		if poster != "" {
			parsed, err := url.Parse(poster)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return nil, badRequest("posterUrl must be an https URL")
			}
		}
		set["posterUrl"] = poster
	}
	if request.Overview != nil {
		if set["overview"], err = cleanText("overview", *request.Overview, false, 4000); err != nil {
			return nil, err
		}
	}
	if request.Note != nil || creating {
		if set["note"], err = cleanText("note", deref(request.Note), false, 1000); err != nil {
			return nil, err
		}
	}
	if request.Status != nil || creating {
		status := deref(request.Status)
		if status == "" {
			status = store.MediaStatusToWatch
		}
		if err := oneOf("status", status, store.MediaStatusToWatch, store.MediaStatusWatching, store.MediaStatusWatched); err != nil {
			return nil, err
		}
		set["status"] = status
	}
	if request.ExternalIDs != nil {
		set["externalIds"] = *request.ExternalIDs
	}
	return set, nil
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	status := r.URL.Query().Get("status")
	if status != "" {
		if err := oneOf("status", status, store.MediaStatusToWatch, store.MediaStatusWatching, store.MediaStatusWatched); err != nil {
			return err
		}
	}
	items, err := s.store.MediaItems(r.Context(), caller.HouseholdID, status)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

func (s *Server) createMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request mediaRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	fields, err := request.fields(true)
	if err != nil {
		return err
	}
	media := &store.Media{
		HouseholdID: caller.HouseholdID,
		Kind:        fields["kind"].(string),
		Title:       fields["title"].(string),
		Year:        deref(request.Year),
		Note:        fields["note"].(string),
		Status:      fields["status"].(string),
		AddedBy:     caller.User.ID,
		// Whoever adds a title wants to watch it, unless they say otherwise.
		InterestedUserIDs: []bson.ObjectID{caller.User.ID},
	}
	if poster, ok := fields["posterUrl"].(string); ok {
		media.PosterURL = poster
	}
	if overview, ok := fields["overview"].(string); ok {
		media.Overview = overview
	}
	if request.ExternalIDs != nil {
		media.ExternalIDs = *request.ExternalIDs
	}
	if request.Interested != nil && !*request.Interested {
		media.InterestedUserIDs = []bson.ObjectID{}
	}
	if media.Status == store.MediaStatusWatched {
		now := store.Now()
		media.WatchedAt = &now
	}
	if err := s.store.CreateMedia(r.Context(), media); err != nil {
		return err
	}

	kindLabel, bodyKey := "a movie", "push.media.movie.body"
	if media.Kind == store.MediaKindShow {
		kindLabel, bodyKey = "a show", "push.media.show.body"
	}
	s.notifier.Notify(caller.HouseholdID, caller.User.ID, push.Notification{
		Title:       "Watchlist",
		Body:        fmt.Sprintf("%s added %s: %s", caller.User.DisplayName, kindLabel, media.Title),
		TitleLocKey: "push.media.title",
		BodyLocKey:  bodyKey,
		BodyLocArgs: []string{caller.User.DisplayName, media.Title},
		ThreadID:    "watchlist",
		Data:        map[string]string{"kind": "media", "mediaId": media.ID.Hex()},
	})
	writeJSON(w, http.StatusCreated, media)
	return nil
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	mediaID, err := pathID(r, "mediaId")
	if err != nil {
		return err
	}
	var request mediaRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	set, err := request.fields(false)
	if err != nil {
		return err
	}
	update := bson.M{"$set": set}
	if status, ok := set["status"].(string); ok {
		if status == store.MediaStatusWatched {
			set["watchedAt"] = store.Now()
		} else {
			update["$unset"] = bson.M{"watchedAt": ""}
		}
	}
	if request.Interested != nil {
		if *request.Interested {
			update["$addToSet"] = bson.M{"interestedUserIds": caller.User.ID}
		} else {
			update["$pull"] = bson.M{"interestedUserIds": caller.User.ID}
		}
	}
	media, err := s.store.UpdateMedia(r.Context(), caller.HouseholdID, mediaID, update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, media)
	return nil
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request, caller principal) error {
	mediaID, err := pathID(r, "mediaId")
	if err != nil {
		return err
	}
	if err := s.store.DeleteMedia(r.Context(), caller.HouseholdID, mediaID); err != nil {
		return err
	}
	noContent(w)
	return nil
}
