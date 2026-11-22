package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familyapp/internal/jellyfin"
	"familyapp/internal/store"
	"familyapp/internal/tmdb"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The Jellyfin sync is one-way: it reads the household's Jellyfin account and
// updates the watchlist. It never writes to Jellyfin.
//
// - A watchlist title matches a Jellyfin movie or show by TMDB id, then TVDB
//   id, then normalized title + year (the rule of the TMDB matching).
// - Played in Jellyfin moves the status forward (To watch → Watching →
//   Watched), never back. A status set by a user is kept until the Jellyfin
//   played state changes.
// - Fully played movies and shows that are not in the watchlist are added as
//   Watched, unless the household deleted them before.

// jellyfinSyncInterval is the time between two automatic syncs.
var jellyfinSyncInterval = 15 * time.Minute

// jellyfinAutoSync turns the background sync on. Tests turn it off and call
// the sync themselves.
var jellyfinAutoSync = true

// jellyfinSyncTimeout limits one sync (every household).
const jellyfinSyncTimeout = 3 * time.Minute

var errSyncRunning = &apiError{Status: http.StatusConflict, Code: "sync_running", Message: "a Jellyfin sync is already running"}

// jellyfinSyncResult is the outcome of one sync, shown in the admin page.
type jellyfinSyncResult struct {
	At      time.Time `json:"at"`
	OK      bool      `json:"ok"`
	Message string    `json:"message,omitempty"`
	// Households is how many households were synced.
	Households int `json:"households"`
	// Available is how many Jellyfin movies and shows were read.
	Available int `json:"available"`
	// Matched is how many watchlist titles are linked to a Jellyfin item.
	Matched        int   `json:"matched"`
	MarkedWatching int   `json:"markedWatching"`
	MarkedWatched  int   `json:"markedWatched"`
	Imported       int   `json:"imported"`
	Gone           int64 `json:"gone"`
	DurationMS     int64 `json:"durationMs"`
}

// jellyfinClient returns the Jellyfin client in use, or nil when not set up.
func (s *Server) jellyfinClient() *jellyfin.Client {
	return s.jellyfin.Load()
}

// jellyfinError maps Jellyfin errors to API errors.
func jellyfinError(err error) error {
	if errors.Is(err, jellyfin.ErrUpstream) {
		return &apiError{Status: http.StatusBadGateway, Code: "upstream", Message: err.Error()}
	}
	return err
}

// --- Background sync -------------------------------------------------------------

// startJellyfinSync syncs at start, every jellyfinSyncInterval, and when asked
// (settings saved, household linked). It also fills the posters of the titles
// it imported, from TMDB.
func (s *Server) startJellyfinSync() {
	if !jellyfinAutoSync {
		return
	}
	s.jellyfinTrigger = make(chan struct{}, 1)
	s.jellyfinStop = make(chan struct{})
	s.jellyfinDone = make(chan struct{})
	go func() {
		defer close(s.jellyfinDone)
		ticker := time.NewTicker(jellyfinSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.jellyfinStop:
				return
			case <-ticker.C:
			case <-s.jellyfinTrigger:
			}
			if s.jellyfinClient() != nil {
				ctx, cancel := context.WithTimeout(context.Background(), jellyfinSyncTimeout)
				s.syncJellyfin(ctx)
				cancel()
			}
			s.fillImportedPosters()
		}
	}()
	s.triggerJellyfin()
}

// triggerJellyfin asks for a sync (it never blocks).
func (s *Server) triggerJellyfin() {
	if s.jellyfinTrigger == nil {
		return
	}
	select {
	case s.jellyfinTrigger <- struct{}{}:
	default: // a run is already pending
	}
}

// syncJellyfin syncs every linked household. It returns false when another
// sync is running.
func (s *Server) syncJellyfin(ctx context.Context) (*jellyfinSyncResult, bool) {
	if !s.jellyfinRunning.CompareAndSwap(false, true) {
		return nil, false
	}
	defer s.jellyfinRunning.Store(false)
	start := time.Now()
	result := &jellyfinSyncResult{At: store.Now()}
	if err := s.syncJellyfinHouseholds(ctx, result); err != nil {
		result.Message = strings.TrimPrefix(err.Error(), jellyfin.ErrUpstream.Error()+": ")
		s.log.Warn("Jellyfin sync failed", "error", err)
	} else {
		result.OK = true
		if result.Households == 0 {
			result.Message = "no household is linked to a Jellyfin user"
		}
		s.log.Info("Jellyfin sync", "households", result.Households, "matched", result.Matched,
			"watched", result.MarkedWatched, "imported", result.Imported)
	}
	result.DurationMS = time.Since(start).Milliseconds()
	s.lastJellyfinSync.Store(result)
	return result, true
}

func (s *Server) syncJellyfinHouseholds(ctx context.Context, result *jellyfinSyncResult) error {
	client := s.jellyfinClient()
	if client == nil {
		return errors.New("Jellyfin is not set up")
	}
	households, err := s.store.JellyfinHouseholds(ctx)
	if err != nil {
		return err
	}
	// Households that share a Jellyfin account read it once.
	libraries := make(map[string][]jellyfin.Item)
	for _, household := range households {
		items, ok := libraries[household.JellyfinUserID]
		if !ok {
			if items, err = client.Library(ctx, household.JellyfinUserID); err != nil {
				return err
			}
			libraries[household.JellyfinUserID] = items
		}
		if err := s.syncHousehold(ctx, household, items, result); err != nil {
			return err
		}
		result.Households++
		result.Available += len(items)
	}
	return nil
}

// syncHousehold links the household's watchlist to the Jellyfin items, moves
// statuses forward and imports the played items it does not have.
func (s *Server) syncHousehold(ctx context.Context, household store.Household, items []jellyfin.Item, result *jellyfinSyncResult) error {
	titles, err := s.store.MediaItems(ctx, household.ID, "")
	if err != nil {
		return err
	}
	index := newJellyfinIndex(items)
	now := store.Now()
	linked := make(map[string]bool, len(titles))
	keep := make([]bson.ObjectID, 0, len(titles))
	for _, title := range titles {
		item := index.find(title)
		if item == nil {
			continue
		}
		linked[item.ID] = true
		keep = append(keep, title.ID)
		result.Matched++
		set := jellyfinChanges(title, *item, now)
		if len(set) == 0 {
			continue
		}
		switch set["status"] {
		case store.MediaStatusWatched:
			result.MarkedWatched++
		case store.MediaStatusWatching:
			result.MarkedWatching++
		}
		if _, err := s.store.UpdateMedia(ctx, household.ID, title.ID, bson.M{"$set": set}); err != nil {
			return err
		}
	}
	gone, err := s.store.MarkJellyfinGone(ctx, household.ID, keep)
	if err != nil {
		return err
	}
	result.Gone += gone

	ignored := make(map[string]bool, len(household.JellyfinIgnoredItems))
	for _, itemID := range household.JellyfinIgnoredItems {
		ignored[itemID] = true
	}
	for _, item := range items {
		if !item.Played || linked[item.ID] || ignored[item.ID] {
			continue
		}
		if err := s.store.CreateMedia(ctx, importedMedia(household.ID, item, now)); err != nil {
			return err
		}
		linked[item.ID] = true
		result.Imported++
	}
	return nil
}

// jellyfinState is the played state of an item.
func jellyfinState(item jellyfin.Item) string {
	switch {
	case item.Played:
		return store.JellyfinStateWatched
	case item.Partly:
		return store.JellyfinStateWatching
	default:
		return store.JellyfinStateNone
	}
}

// statusRank orders the statuses: the sync only moves a status up.
func statusRank(status string) int {
	switch status {
	case store.MediaStatusWatching:
		return 1
	case store.MediaStatusWatched:
		return 2
	default:
		return 0
	}
}

// jellyfinChanges returns the fields to set on a matched title, or an empty
// map when nothing changed.
func jellyfinChanges(title store.Media, item jellyfin.Item, now time.Time) bson.M {
	state := jellyfinState(item)
	previous := store.MediaJellyfin{}
	if title.Jellyfin != nil {
		previous = *title.Jellyfin
	}
	set := bson.M{}
	if title.Jellyfin == nil || !previous.Available || previous.ItemID != item.ID || previous.State != state || !sameDate(previous.PlayedAt, item.PlayedAt) {
		set["jellyfin"] = store.MediaJellyfin{ItemID: item.ID, Available: true, PlayedAt: item.PlayedAt, State: state, SyncedAt: now}
	}
	// Ids from Jellyfin let the details page and Radarr/Sonarr work.
	if title.ExternalIDs.TMDB == 0 && item.TMDB > 0 {
		set["externalIds.tmdb"] = item.TMDB
		set["matchCheckedAt"] = now
	}
	if title.ExternalIDs.TVDB == 0 && item.TVDB > 0 {
		set["externalIds.tvdb"] = item.TVDB
	}

	// A user's choice wins while Jellyfin says the same thing as before.
	keptByUser := title.StatusSetBy == store.StatusSetByUser && title.Jellyfin != nil && previous.State == state
	target := store.MediaStatusToWatch
	switch state {
	case store.JellyfinStateWatched:
		target = store.MediaStatusWatched
	case store.JellyfinStateWatching:
		target = store.MediaStatusWatching
	}
	if !keptByUser && statusRank(target) > statusRank(title.Status) {
		set["status"] = target
		set["statusSetBy"] = store.StatusSetByJellyfin
		if target == store.MediaStatusWatched {
			watchedAt := now
			if item.PlayedAt != nil {
				watchedAt = *item.PlayedAt
			}
			set["watchedAt"] = watchedAt
		}
	}
	return set
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// importedMedia is a watchlist title made from a played Jellyfin item.
func importedMedia(householdID bson.ObjectID, item jellyfin.Item, now time.Time) *store.Media {
	watchedAt := now
	if item.PlayedAt != nil {
		watchedAt = *item.PlayedAt
	}
	return &store.Media{
		HouseholdID:       householdID,
		Kind:              item.Kind,
		Title:             item.Title,
		Year:              item.Year,
		Status:            store.MediaStatusWatched,
		InterestedUserIDs: []bson.ObjectID{},
		ExternalIDs:       store.ExternalIDs{TMDB: item.TMDB, TVDB: item.TVDB},
		WatchedAt:         &watchedAt,
		Source:            store.SourceJellyfin,
		StatusSetBy:       store.StatusSetByJellyfin,
		Jellyfin:          &store.MediaJellyfin{ItemID: item.ID, Available: true, PlayedAt: item.PlayedAt, State: store.JellyfinStateWatched, SyncedAt: now},
	}
}

// fillImportedPosters gets the poster (and IMDb id, for Radarr) of the titles
// the sync imported, from TMDB. Each title is looked at once.
func (s *Server) fillImportedPosters() {
	client := s.tmdbClient()
	if client == nil {
		return
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		pending, err := s.store.ImportedWithoutDetails(ctx, store.SourceJellyfin, 50)
		cancel()
		if err != nil {
			s.log.Warn("Jellyfin posters: load titles", "error", err)
			return
		}
		if len(pending) == 0 {
			return
		}
		for _, media := range pending {
			select {
			case <-s.jellyfinStop:
				return
			default:
			}
			if err := s.fillPoster(client, media); err != nil {
				// TMDB down or key wrong: try again at the next sync.
				s.log.Warn("Jellyfin posters stopped", "error", err)
				return
			}
			time.Sleep(matchDelay)
		}
	}
}

func (s *Server) fillPoster(client *tmdb.Client, media store.Media) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	set := bson.M{"matchCheckedAt": store.Now()}
	details, err := s.cachedDetails(ctx, client, media.Kind, media.ExternalIDs.TMDB, "en-US")
	switch {
	case errors.Is(err, tmdb.ErrUpstream) && strings.Contains(err.Error(), "404"):
		// Unknown at TMDB: keep the title without a poster.
	case err != nil:
		return err
	default:
		if details.PosterURL != "" {
			set["posterUrl"] = details.PosterURL
		}
		if details.IMDBID != "" {
			set["externalIds.imdb"] = details.IMDBID
		}
	}
	return s.store.SetMediaMatch(ctx, media.ID, set)
}

// --- Matching ------------------------------------------------------------------

// jellyfinIndex finds the Jellyfin item of a watchlist title.
type jellyfinIndex struct {
	byTMDB  map[string]*jellyfin.Item
	byTVDB  map[string]*jellyfin.Item
	byTitle map[string][]*jellyfin.Item
}

func newJellyfinIndex(items []jellyfin.Item) jellyfinIndex {
	index := jellyfinIndex{
		byTMDB:  make(map[string]*jellyfin.Item, len(items)),
		byTVDB:  make(map[string]*jellyfin.Item, len(items)),
		byTitle: make(map[string][]*jellyfin.Item, len(items)),
	}
	for i := range items {
		item := &items[i]
		if item.TMDB > 0 {
			index.byTMDB[idKey(item.Kind, item.TMDB)] = item
		}
		if item.TVDB > 0 {
			index.byTVDB[idKey(item.Kind, item.TVDB)] = item
		}
		key := item.Kind + ":" + tmdb.Normalize(item.Title)
		index.byTitle[key] = append(index.byTitle[key], item)
	}
	return index
}

func idKey(kind string, id int) string {
	return kind + ":" + strconv.Itoa(id)
}

// find returns the item of a title: by TMDB id, then TVDB id, then same
// normalized title with a year at most one year apart. A title match is
// refused when both have a different TMDB id (a remake).
func (index jellyfinIndex) find(title store.Media) *jellyfin.Item {
	if title.ExternalIDs.TMDB > 0 {
		if item := index.byTMDB[idKey(title.Kind, title.ExternalIDs.TMDB)]; item != nil {
			return item
		}
	}
	if title.ExternalIDs.TVDB > 0 {
		if item := index.byTVDB[idKey(title.Kind, title.ExternalIDs.TVDB)]; item != nil {
			return item
		}
	}
	normalized := tmdb.Normalize(title.Title)
	if normalized == "" {
		return nil
	}
	for _, item := range index.byTitle[title.Kind+":"+normalized] {
		if title.ExternalIDs.TMDB > 0 && item.TMDB > 0 && title.ExternalIDs.TMDB != item.TMDB {
			continue
		}
		if title.Year == 0 || item.Year == 0 || abs(title.Year-item.Year) <= 1 {
			return item
		}
	}
	return nil
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// --- Admin routes ----------------------------------------------------------------

// adminJellyfinState is the Jellyfin card of the admin page.
type adminJellyfinState struct {
	Configured bool `json:"configured"`
	// JellyfinUserID is the Jellyfin account of the admin's household.
	JellyfinUserID string              `json:"jellyfinUserId"`
	Running        bool                `json:"running"`
	LastSync       *jellyfinSyncResult `json:"lastSync"`
}

func (s *Server) jellyfinState(ctx context.Context, householdID bson.ObjectID) (adminJellyfinState, error) {
	household, err := s.store.HouseholdByID(ctx, householdID)
	if err != nil {
		return adminJellyfinState{}, err
	}
	return adminJellyfinState{
		Configured:     s.jellyfinClient() != nil,
		JellyfinUserID: household.JellyfinUserID,
		Running:        s.jellyfinRunning.Load(),
		LastSync:       s.lastJellyfinSync.Load(),
	}, nil
}

func (s *Server) adminJellyfin(w http.ResponseWriter, r *http.Request, caller principal) error {
	state, err := s.jellyfinState(r.Context(), caller.HouseholdID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, state)
	return nil
}

// adminJellyfinUsers lists the Jellyfin accounts, to pick the household's one.
func (s *Server) adminJellyfinUsers(w http.ResponseWriter, r *http.Request, caller principal) error {
	client := s.jellyfinClient()
	if client == nil {
		return notConfigured("Jellyfin")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	users, err := client.Users(ctx)
	if err != nil {
		return jellyfinError(err)
	}
	writeJSON(w, http.StatusOK, users)
	return nil
}

type jellyfinHouseholdRequest struct {
	JellyfinUserID *string `json:"jellyfinUserId"`
}

// adminSetJellyfinUser links the admin's household to a Jellyfin account
// ("" removes the link), then starts a sync.
func (s *Server) adminSetJellyfinUser(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request jellyfinHouseholdRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if request.JellyfinUserID == nil {
		return badRequest("jellyfinUserId is required")
	}
	userID := strings.TrimSpace(*request.JellyfinUserID)
	if userID != "" {
		client := s.jellyfinClient()
		if client == nil {
			return notConfigured("Jellyfin")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		users, err := client.Users(ctx)
		if err != nil {
			return jellyfinError(err)
		}
		known := false
		for _, user := range users {
			known = known || user.ID == userID
		}
		if !known {
			return badRequest("this Jellyfin user does not exist")
		}
	}
	if _, err := s.store.SetHouseholdJellyfinUser(r.Context(), caller.HouseholdID, userID); err != nil {
		return err
	}
	s.log.Info("household Jellyfin user changed", "by", caller.User.Username, "linked", userID != "")
	if userID != "" {
		s.triggerJellyfin()
	}
	state, err := s.jellyfinState(r.Context(), caller.HouseholdID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, state)
	return nil
}

// adminSyncJellyfin runs a sync now and returns its result.
func (s *Server) adminSyncJellyfin(w http.ResponseWriter, r *http.Request, caller principal) error {
	if s.jellyfinClient() == nil {
		return notConfigured("Jellyfin")
	}
	ctx, cancel := context.WithTimeout(r.Context(), jellyfinSyncTimeout)
	defer cancel()
	result, ran := s.syncJellyfin(ctx)
	if !ran {
		return errSyncRunning
	}
	if result.Imported > 0 {
		// The posters of the new titles come from TMDB, in the background.
		s.triggerJellyfin()
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}
