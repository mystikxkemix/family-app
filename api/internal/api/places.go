package api

import (
	"fmt"
	"net/http"
	"net/url"

	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type placeRequest struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Address  *string `json:"address"`
	URL      *string `json:"url"`
	Note     *string `json:"note"`
	Status   *string `json:"status"`
	Rating   *int    `json:"rating"`
	// Interested adds (true) or removes (false) the caller from the people who
	// want to go.
	Interested *bool `json:"interested"`
}

// fields validates the request and returns the fields it sets.
func (request placeRequest) fields(creating bool) (bson.M, error) {
	set := bson.M{}
	var err error
	if request.Name != nil || creating {
		if set["name"], err = cleanText("name", deref(request.Name), true, 120); err != nil {
			return nil, err
		}
	}
	if request.Category != nil || creating {
		category := deref(request.Category)
		if category == "" {
			category = store.PlaceCategoryRestaurant
		}
		if err := oneOf("category", category, store.PlaceCategories...); err != nil {
			return nil, err
		}
		set["category"] = category
	}
	if request.Address != nil || creating {
		if set["address"], err = cleanText("address", deref(request.Address), false, 300); err != nil {
			return nil, err
		}
	}
	if request.URL != nil || creating {
		link, err := cleanText("url", deref(request.URL), false, 1000)
		if err != nil {
			return nil, err
		}
		if link != "" {
			parsed, err := url.Parse(link)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return nil, badRequest("url must be a web address")
			}
		}
		set["url"] = link
	}
	if request.Note != nil || creating {
		if set["note"], err = cleanText("note", deref(request.Note), false, 1000); err != nil {
			return nil, err
		}
	}
	if request.Status != nil || creating {
		status := deref(request.Status)
		if status == "" {
			status = store.PlaceStatusToTry
		}
		if err := oneOf("status", status, store.PlaceStatusToTry, store.PlaceStatusTried); err != nil {
			return nil, err
		}
		set["status"] = status
	}
	if request.Rating != nil {
		if *request.Rating < 0 || *request.Rating > 5 {
			return nil, badRequest("rating must be between 0 and 5")
		}
		set["rating"] = *request.Rating
	}
	return set, nil
}

func (s *Server) listPlaces(w http.ResponseWriter, r *http.Request, caller principal) error {
	status := r.URL.Query().Get("status")
	if status != "" {
		if err := oneOf("status", status, store.PlaceStatusToTry, store.PlaceStatusTried); err != nil {
			return err
		}
	}
	places, err := s.store.Places(r.Context(), caller.HouseholdID, status)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, places)
	return nil
}

func (s *Server) createPlace(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request placeRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	fields, err := request.fields(true)
	if err != nil {
		return err
	}
	place := &store.Place{
		HouseholdID: caller.HouseholdID,
		Name:        fields["name"].(string),
		Category:    fields["category"].(string),
		Address:     fields["address"].(string),
		URL:         fields["url"].(string),
		Note:        fields["note"].(string),
		Status:      fields["status"].(string),
		Rating:      deref(request.Rating),
		AddedBy:     caller.User.ID,
		// Whoever adds a place wants to go, unless they say otherwise.
		InterestedUserIDs: []bson.ObjectID{caller.User.ID},
	}
	if request.Interested != nil && !*request.Interested {
		place.InterestedUserIDs = []bson.ObjectID{}
	}
	if place.Status == store.PlaceStatusTried {
		now := store.Now()
		place.TriedAt = &now
	}
	if err := s.store.CreatePlace(r.Context(), place); err != nil {
		return err
	}
	s.notifier.Notify(caller.HouseholdID, caller.User.ID, push.Notification{
		Title:       "Places to try",
		Body:        fmt.Sprintf("%s added a place: %s", caller.User.DisplayName, place.Name),
		TitleLocKey: "push.place.title",
		BodyLocKey:  "push.place.body",
		BodyLocArgs: []string{caller.User.DisplayName, place.Name},
		ThreadID:    "places",
		Data:        map[string]string{"kind": "place", "placeId": place.ID.Hex()},
	})
	writeJSON(w, http.StatusCreated, place)
	return nil
}

func (s *Server) updatePlace(w http.ResponseWriter, r *http.Request, caller principal) error {
	placeID, err := pathID(r, "placeId")
	if err != nil {
		return err
	}
	var request placeRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	set, err := request.fields(false)
	if err != nil {
		return err
	}
	update := bson.M{"$set": set}
	if status, ok := set["status"].(string); ok {
		if status == store.PlaceStatusTried {
			set["triedAt"] = store.Now()
		} else {
			update["$unset"] = bson.M{"triedAt": ""}
		}
	}
	addInterest(update, request.Interested, caller.User.ID)
	place, err := s.store.UpdatePlace(r.Context(), caller.HouseholdID, placeID, update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, place)
	return nil
}

func (s *Server) deletePlace(w http.ResponseWriter, r *http.Request, caller principal) error {
	placeID, err := pathID(r, "placeId")
	if err != nil {
		return err
	}
	if err := s.store.DeletePlace(r.Context(), caller.HouseholdID, placeID); err != nil {
		return err
	}
	noContent(w)
	return nil
}

// addInterest adds the caller to (or removes them from) interestedUserIds.
// Shared by the watchlist and the places.
func addInterest(update bson.M, interested *bool, userID bson.ObjectID) {
	if interested == nil {
		return
	}
	if *interested {
		update["$addToSet"] = bson.M{"interestedUserIds": userID}
	} else {
		update["$pull"] = bson.M{"interestedUserIds": userID}
	}
}
