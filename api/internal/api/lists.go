package api

import (
	"fmt"
	"net/http"
	"strconv"

	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- Lists --------------------------------------------------------------------

type listRequest struct {
	Name  *string `json:"name"`
	Kind  *string `json:"kind"`
	Emoji *string `json:"emoji"`
}

// fields validates the request and returns the fields it sets.
func (request listRequest) fields(creating bool) (bson.M, error) {
	set := bson.M{}
	if request.Name != nil || creating {
		name, err := cleanText("name", deref(request.Name), true, 60)
		if err != nil {
			return nil, err
		}
		set["name"] = name
	}
	if request.Kind != nil || creating {
		kind := deref(request.Kind)
		if kind == "" {
			kind = store.ListKindOther
		}
		if err := oneOf("kind", kind, store.ListKindGrocery, store.ListKindTodo, store.ListKindOther); err != nil {
			return nil, err
		}
		set["kind"] = kind
	}
	if request.Emoji != nil || creating {
		emoji, err := cleanText("emoji", deref(request.Emoji), false, 8)
		if err != nil {
			return nil, err
		}
		set["emoji"] = emoji
	}
	return set, nil
}

func (s *Server) listLists(w http.ResponseWriter, r *http.Request, caller principal) error {
	lists, err := s.store.Lists(r.Context(), caller.HouseholdID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, lists)
	return nil
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request listRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	fields, err := request.fields(true)
	if err != nil {
		return err
	}
	list := &store.List{
		HouseholdID: caller.HouseholdID,
		Name:        fields["name"].(string),
		Kind:        fields["kind"].(string),
		Emoji:       fields["emoji"].(string),
		CreatedBy:   caller.User.ID,
	}
	if err := s.store.CreateList(r.Context(), list); err != nil {
		return err
	}
	s.notifier.Notify(caller.HouseholdID, caller.User.ID, push.Notification{
		Title:       "New list",
		Body:        fmt.Sprintf("%s created the list \"%s\"", caller.User.DisplayName, list.Name),
		TitleLocKey: "push.list.created.title",
		BodyLocKey:  "push.list.created.body",
		BodyLocArgs: []string{caller.User.DisplayName, list.Name},
		ThreadID:    "lists",
		Data:        map[string]string{"kind": "list", "listId": list.ID.Hex()},
	})
	writeJSON(w, http.StatusCreated, list)
	return nil
}

func (s *Server) updateList(w http.ResponseWriter, r *http.Request, caller principal) error {
	listID, err := pathID(r, "listId")
	if err != nil {
		return err
	}
	var request listRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	fields, err := request.fields(false)
	if err != nil {
		return err
	}
	list, err := s.store.UpdateList(r.Context(), caller.HouseholdID, listID, fields)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request, caller principal) error {
	listID, err := pathID(r, "listId")
	if err != nil {
		return err
	}
	if err := s.store.DeleteList(r.Context(), caller.HouseholdID, listID); err != nil {
		return err
	}
	noContent(w)
	return nil
}

// --- List items ---------------------------------------------------------------

type listItemRequest struct {
	Text    *string `json:"text"`
	Note    *string `json:"note"`
	Checked *bool   `json:"checked"`
}

// requireList loads the list of the path, checking it is in the household.
func (s *Server) requireList(r *http.Request, caller principal) (*store.List, error) {
	listID, err := pathID(r, "listId")
	if err != nil {
		return nil, err
	}
	return s.store.ListByID(r.Context(), caller.HouseholdID, listID)
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request, caller principal) error {
	list, err := s.requireList(r, caller)
	if err != nil {
		return err
	}
	items, err := s.store.ListItems(r.Context(), caller.HouseholdID, list.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items)
	return nil
}

func (s *Server) createListItem(w http.ResponseWriter, r *http.Request, caller principal) error {
	list, err := s.requireList(r, caller)
	if err != nil {
		return err
	}
	var request listItemRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	text, err := cleanText("text", deref(request.Text), true, 200)
	if err != nil {
		return err
	}
	note, err := cleanText("note", deref(request.Note), false, 500)
	if err != nil {
		return err
	}
	item := &store.ListItem{
		HouseholdID: caller.HouseholdID,
		ListID:      list.ID,
		Text:        text,
		Note:        note,
		CreatedBy:   caller.User.ID,
	}
	if err := s.store.CreateListItem(r.Context(), item); err != nil {
		return err
	}

	// One push per burst of additions, not one per item.
	actor := caller.User.DisplayName
	listName := list.Name
	batchKey := "list-items:" + list.ID.Hex() + ":" + caller.User.ID.Hex()
	s.notifier.NotifyBatched(batchKey, caller.HouseholdID, caller.User.ID, func(count int) push.Notification {
		body := fmt.Sprintf("%s added \"%s\"", actor, text)
		bodyKey, bodyArgs := "push.items.added.one", []string{actor, text}
		if count > 1 {
			body = fmt.Sprintf("%s added %d items", actor, count)
			bodyKey, bodyArgs = "push.items.added.many", []string{actor, strconv.Itoa(count)}
		}
		return push.Notification{
			Title:       listName,
			Body:        body,
			BodyLocKey:  bodyKey,
			BodyLocArgs: bodyArgs,
			ThreadID:    "list-" + list.ID.Hex(),
			Data:        map[string]string{"kind": "list", "listId": list.ID.Hex()},
		}
	})
	writeJSON(w, http.StatusCreated, item)
	return nil
}

func (s *Server) updateListItem(w http.ResponseWriter, r *http.Request, caller principal) error {
	list, err := s.requireList(r, caller)
	if err != nil {
		return err
	}
	itemID, err := pathID(r, "itemId")
	if err != nil {
		return err
	}
	var request listItemRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}

	set, unset := bson.M{}, bson.M{}
	if request.Text != nil {
		if set["text"], err = cleanText("text", *request.Text, true, 200); err != nil {
			return err
		}
	}
	if request.Note != nil {
		if set["note"], err = cleanText("note", *request.Note, false, 500); err != nil {
			return err
		}
	}
	if request.Checked != nil {
		set["checked"] = *request.Checked
		if *request.Checked {
			set["checkedBy"] = caller.User.ID
			set["checkedAt"] = store.Now()
		} else {
			unset["checkedBy"] = ""
			unset["checkedAt"] = ""
		}
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	item, err := s.store.UpdateListItem(r.Context(), caller.HouseholdID, list.ID, itemID, update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

func (s *Server) deleteListItem(w http.ResponseWriter, r *http.Request, caller principal) error {
	list, err := s.requireList(r, caller)
	if err != nil {
		return err
	}
	itemID, err := pathID(r, "itemId")
	if err != nil {
		return err
	}
	if err := s.store.DeleteListItem(r.Context(), caller.HouseholdID, list.ID, itemID); err != nil {
		return err
	}
	noContent(w)
	return nil
}

func (s *Server) clearCheckedItems(w http.ResponseWriter, r *http.Request, caller principal) error {
	list, err := s.requireList(r, caller)
	if err != nil {
		return err
	}
	deleted, err := s.store.DeleteCheckedItems(r.Context(), caller.HouseholdID, list.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": deleted})
	return nil
}

// deref returns the value of a pointer, or the zero value for nil.
func deref[T any](pointer *T) T {
	var zero T
	if pointer == nil {
		return zero
	}
	return *pointer
}
