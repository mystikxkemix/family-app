package api

import (
	"fmt"
	"net/http"
	"time"

	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type eventRequest struct {
	Title    *string    `json:"title"`
	StartsAt *time.Time `json:"startsAt"`
	// EndsAt is optional. Send null with clearEnd to remove it.
	EndsAt   *time.Time `json:"endsAt"`
	ClearEnd bool       `json:"clearEnd"`
	AllDay   *bool      `json:"allDay"`
	Location *string    `json:"location"`
	Notes    *string    `json:"notes"`
}

// fields validates the request and returns the fields it sets and unsets.
func (request eventRequest) fields(creating bool) (set, unset bson.M, err error) {
	set, unset = bson.M{}, bson.M{}
	if request.Title != nil || creating {
		if set["title"], err = cleanText("title", deref(request.Title), true, 120); err != nil {
			return nil, nil, err
		}
	}
	if request.StartsAt != nil {
		set["startsAt"] = request.StartsAt.UTC().Truncate(time.Millisecond)
	} else if creating {
		return nil, nil, badRequest("startsAt is required")
	}
	if request.EndsAt != nil {
		set["endsAt"] = request.EndsAt.UTC().Truncate(time.Millisecond)
	} else if request.ClearEnd {
		unset["endsAt"] = ""
	}
	if request.AllDay != nil || creating {
		set["allDay"] = deref(request.AllDay)
	}
	if request.Location != nil || creating {
		if set["location"], err = cleanText("location", deref(request.Location), false, 200); err != nil {
			return nil, nil, err
		}
	}
	if request.Notes != nil || creating {
		if set["notes"], err = cleanText("notes", deref(request.Notes), false, 2000); err != nil {
			return nil, nil, err
		}
	}
	starts, hasStart := set["startsAt"].(time.Time)
	ends, hasEnd := set["endsAt"].(time.Time)
	if hasStart && hasEnd && ends.Before(starts) {
		return nil, nil, badRequest("endsAt must be after startsAt")
	}
	return set, unset, nil
}

// listEvents returns the events from "from" (RFC 3339), by default from 24
// hours ago, so today's events stay visible.
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request, caller principal) error {
	from := time.Now().Add(-24 * time.Hour)
	if value := r.URL.Query().Get("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return badRequest("from must be an RFC 3339 date")
		}
		from = parsed
	}
	events, err := s.store.Events(r.Context(), caller.HouseholdID, from.UTC())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, events)
	return nil
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request eventRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	set, _, err := request.fields(true)
	if err != nil {
		return err
	}
	event := &store.Event{
		HouseholdID: caller.HouseholdID,
		Title:       set["title"].(string),
		StartsAt:    set["startsAt"].(time.Time),
		AllDay:      set["allDay"].(bool),
		Location:    set["location"].(string),
		Notes:       set["notes"].(string),
		CreatedBy:   caller.User.ID,
	}
	if ends, ok := set["endsAt"].(time.Time); ok {
		event.EndsAt = &ends
	}
	if err := s.store.CreateEvent(r.Context(), event); err != nil {
		return err
	}
	s.notifier.Notify(caller.HouseholdID, caller.User.ID, push.Notification{
		Title:       "New event",
		Body:        fmt.Sprintf("%s added \"%s\"", caller.User.DisplayName, event.Title),
		TitleLocKey: "push.event.created.title",
		BodyLocKey:  "push.event.created.body",
		BodyLocArgs: []string{caller.User.DisplayName, event.Title},
		ThreadID:    "events",
		Data:        map[string]string{"kind": "event", "eventId": event.ID.Hex()},
	})
	writeJSON(w, http.StatusCreated, event)
	return nil
}

func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request, caller principal) error {
	eventID, err := pathID(r, "eventId")
	if err != nil {
		return err
	}
	var request eventRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	set, unset, err := request.fields(false)
	if err != nil {
		return err
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	event, err := s.store.UpdateEvent(r.Context(), caller.HouseholdID, eventID, update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, event)
	return nil
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request, caller principal) error {
	eventID, err := pathID(r, "eventId")
	if err != nil {
		return err
	}
	if err := s.store.DeleteEvent(r.Context(), caller.HouseholdID, eventID); err != nil {
		return err
	}
	noContent(w)
	return nil
}
