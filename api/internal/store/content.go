package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// --- Lists --------------------------------------------------------------------

// Lists returns the household lists with their open and total item counts.
func (s *Store) Lists(ctx context.Context, householdID bson.ObjectID) ([]List, error) {
	lists, err := findAll[List](ctx, s.lists, bson.M{"householdId": householdID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil || len(lists) == 0 {
		return lists, err
	}

	cursor, err := s.listItems.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"householdId": householdID}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$listId",
			"total": bson.M{"$sum": 1},
			"open":  bson.M{"$sum": bson.M{"$cond": bson.A{"$checked", 0, 1}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	var counts []struct {
		ListID bson.ObjectID `bson:"_id"`
		Total  int64         `bson:"total"`
		Open   int64         `bson:"open"`
	}
	if err := cursor.All(ctx, &counts); err != nil {
		return nil, err
	}

	indexByID := make(map[bson.ObjectID]int, len(lists))
	for i := range lists {
		indexByID[lists[i].ID] = i
	}
	for _, count := range counts {
		if i, ok := indexByID[count.ListID]; ok {
			lists[i].TotalCount = count.Total
			lists[i].OpenCount = count.Open
		}
	}
	return lists, nil
}

// ListByID returns one list of a household.
func (s *Store) ListByID(ctx context.Context, householdID, id bson.ObjectID) (*List, error) {
	return findOne[List](ctx, s.lists, scoped(householdID, id))
}

// CreateList inserts a list and fills its ID and dates.
func (s *Store) CreateList(ctx context.Context, list *List) error {
	now := Now()
	list.ID = bson.NewObjectID()
	list.CreatedAt = now
	list.UpdatedAt = now
	return insertOne(ctx, s.lists, list)
}

// UpdateList sets fields on a list and returns it.
func (s *Store) UpdateList(ctx context.Context, householdID, id bson.ObjectID, set bson.M) (*List, error) {
	set["updatedAt"] = Now()
	return updateOne[List](ctx, s.lists, scoped(householdID, id), bson.M{"$set": set})
}

// DeleteList deletes a list and all its items.
func (s *Store) DeleteList(ctx context.Context, householdID, id bson.ObjectID) error {
	if err := deleteOne(ctx, s.lists, scoped(householdID, id)); err != nil {
		return err
	}
	_, err := s.listItems.DeleteMany(ctx, bson.M{"householdId": householdID, "listId": id})
	return err
}

// touchList moves the list "updated" date, so the app can sort lists by activity.
func (s *Store) touchList(ctx context.Context, householdID, id bson.ObjectID) error {
	_, err := s.lists.UpdateOne(ctx, scoped(householdID, id), bson.M{"$set": bson.M{"updatedAt": Now()}})
	return err
}

// --- List items ---------------------------------------------------------------

// ListItems returns the items of a list, oldest first.
func (s *Store) ListItems(ctx context.Context, householdID, listID bson.ObjectID) ([]ListItem, error) {
	return findAll[ListItem](ctx, s.listItems, bson.M{"householdId": householdID, "listId": listID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
}

// CreateListItem inserts an item and fills its ID and dates.
func (s *Store) CreateListItem(ctx context.Context, item *ListItem) error {
	now := Now()
	item.ID = bson.NewObjectID()
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := insertOne(ctx, s.listItems, item); err != nil {
		return err
	}
	return s.touchList(ctx, item.HouseholdID, item.ListID)
}

// UpdateListItem applies an update to an item of a list and returns it.
func (s *Store) UpdateListItem(ctx context.Context, householdID, listID, id bson.ObjectID, update bson.M) (*ListItem, error) {
	filter := bson.M{"_id": id, "householdId": householdID, "listId": listID}
	item, err := updateOne[ListItem](ctx, s.listItems, filter, withUpdatedAt(update))
	if err != nil {
		return nil, err
	}
	return item, s.touchList(ctx, householdID, listID)
}

// DeleteListItem deletes one item of a list.
func (s *Store) DeleteListItem(ctx context.Context, householdID, listID, id bson.ObjectID) error {
	return deleteOne(ctx, s.listItems, bson.M{"_id": id, "householdId": householdID, "listId": listID})
}

// DeleteCheckedItems deletes the checked items of a list and returns how many.
func (s *Store) DeleteCheckedItems(ctx context.Context, householdID, listID bson.ObjectID) (int64, error) {
	result, err := s.listItems.DeleteMany(ctx, bson.M{"householdId": householdID, "listId": listID, "checked": true})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// --- Watchlist ----------------------------------------------------------------

// MediaItems returns the watchlist, newest first. An empty status returns all.
func (s *Store) MediaItems(ctx context.Context, householdID bson.ObjectID, status string) ([]Media, error) {
	filter := bson.M{"householdId": householdID}
	if status != "" {
		filter["status"] = status
	}
	return findAll[Media](ctx, s.media, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
}

// CreateMedia inserts a watchlist entry and fills its ID and dates.
func (s *Store) CreateMedia(ctx context.Context, media *Media) error {
	now := Now()
	media.ID = bson.NewObjectID()
	media.CreatedAt = now
	media.UpdatedAt = now
	if media.InterestedUserIDs == nil {
		media.InterestedUserIDs = []bson.ObjectID{}
	}
	return insertOne(ctx, s.media, media)
}

// UpdateMedia applies an update to a watchlist entry and returns it.
func (s *Store) UpdateMedia(ctx context.Context, householdID, id bson.ObjectID, update bson.M) (*Media, error) {
	return updateOne[Media](ctx, s.media, scoped(householdID, id), withUpdatedAt(update))
}

// DeleteMedia deletes a watchlist entry.
func (s *Store) DeleteMedia(ctx context.Context, householdID, id bson.ObjectID) error {
	return deleteOne(ctx, s.media, scoped(householdID, id))
}

// --- Places -------------------------------------------------------------------

// Places returns the places to try, newest first. An empty status returns all.
func (s *Store) Places(ctx context.Context, householdID bson.ObjectID, status string) ([]Place, error) {
	filter := bson.M{"householdId": householdID}
	if status != "" {
		filter["status"] = status
	}
	return findAll[Place](ctx, s.places, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
}

// CreatePlace inserts a place and fills its ID and dates.
func (s *Store) CreatePlace(ctx context.Context, place *Place) error {
	now := Now()
	place.ID = bson.NewObjectID()
	place.CreatedAt = now
	place.UpdatedAt = now
	if place.InterestedUserIDs == nil {
		place.InterestedUserIDs = []bson.ObjectID{}
	}
	return insertOne(ctx, s.places, place)
}

// UpdatePlace applies an update to a place and returns it.
func (s *Store) UpdatePlace(ctx context.Context, householdID, id bson.ObjectID, update bson.M) (*Place, error) {
	return updateOne[Place](ctx, s.places, scoped(householdID, id), withUpdatedAt(update))
}

// DeletePlace deletes a place.
func (s *Store) DeletePlace(ctx context.Context, householdID, id bson.ObjectID) error {
	return deleteOne(ctx, s.places, scoped(householdID, id))
}

// MediaByID returns one watchlist entry of a household.
func (s *Store) MediaByID(ctx context.Context, householdID, id bson.ObjectID) (*Media, error) {
	return findOne[Media](ctx, s.media, scoped(householdID, id))
}

// --- TMDB details cache and matching -------------------------------------------

// DetailsCacheTTL is how long TMDB details are kept before being fetched again.
const DetailsCacheTTL = 30 * 24 * time.Hour

// CachedDetails returns the cached TMDB details (JSON) of a title in a
// language, or ErrNotFound.
func (s *Store) CachedDetails(ctx context.Context, kind string, tmdbID int, lang string) ([]byte, error) {
	var entry struct {
		Payload []byte `bson:"payload"`
	}
	err := s.mediaDetails.FindOne(ctx, bson.M{"kind": kind, "tmdbId": tmdbID, "lang": lang}).Decode(&entry)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return entry.Payload, nil
}

// SaveDetails stores TMDB details (JSON) in the cache.
func (s *Store) SaveDetails(ctx context.Context, kind string, tmdbID int, lang string, payload []byte) error {
	_, err := s.mediaDetails.UpdateOne(ctx,
		bson.M{"kind": kind, "tmdbId": tmdbID, "lang": lang},
		bson.M{"$set": bson.M{"payload": payload, "fetchedAt": Now()}},
		options.UpdateOne().SetUpsert(true))
	return err
}

// MediaToMatch returns titles of every household that have no TMDB id and
// were never looked at by the automatic matching.
func (s *Store) MediaToMatch(ctx context.Context, limit int64) ([]Media, error) {
	filter := bson.M{
		"matchCheckedAt": bson.M{"$exists": false},
		"$or": bson.A{
			bson.M{"externalIds.tmdb": bson.M{"$exists": false}},
			bson.M{"externalIds.tmdb": 0},
		},
	}
	return findAll[Media](ctx, s.media, filter, options.Find().SetLimit(limit).SetSort(bson.D{{Key: "createdAt", Value: 1}}))
}

// SetMediaMatch records the result of the automatic matching for a title.
// The set may be empty (no safe match): the title is only marked as checked.
func (s *Store) SetMediaMatch(ctx context.Context, id bson.ObjectID, set bson.M) error {
	set["matchCheckedAt"] = Now()
	_, err := s.media.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// MarkJellyfinGone sets "not available" on the household titles linked to a
// Jellyfin item that the last sync did not see (keep lists the ones it saw).
func (s *Store) MarkJellyfinGone(ctx context.Context, householdID bson.ObjectID, keep []bson.ObjectID) (int64, error) {
	filter := bson.M{"householdId": householdID, "jellyfin.available": true, "_id": bson.M{"$nin": keep}}
	result, err := s.media.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"jellyfin.available": false, "jellyfin.syncedAt": Now()}})
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// ImportedWithoutDetails returns titles added by a sync (source) that have a
// TMDB id but were never looked up at TMDB (no poster yet).
func (s *Store) ImportedWithoutDetails(ctx context.Context, source string, limit int64) ([]Media, error) {
	filter := bson.M{
		"source":           source,
		"externalIds.tmdb": bson.M{"$gt": 0},
		"matchCheckedAt":   bson.M{"$exists": false},
	}
	return findAll[Media](ctx, s.media, filter, options.Find().SetLimit(limit).SetSort(bson.D{{Key: "createdAt", Value: 1}}))
}

// --- Events -------------------------------------------------------------------

// Events returns the events that end (or start, when they have no end) after
// "from", soonest first.
func (s *Store) Events(ctx context.Context, householdID bson.ObjectID, from time.Time) ([]Event, error) {
	filter := bson.M{
		"householdId": householdID,
		"$or": bson.A{
			bson.M{"startsAt": bson.M{"$gte": from}},
			bson.M{"endsAt": bson.M{"$gte": from}},
		},
	}
	return findAll[Event](ctx, s.events, filter, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}}))
}

// CreateEvent inserts an event and fills its ID and dates.
func (s *Store) CreateEvent(ctx context.Context, event *Event) error {
	now := Now()
	event.ID = bson.NewObjectID()
	event.CreatedAt = now
	event.UpdatedAt = now
	return insertOne(ctx, s.events, event)
}

// UpdateEvent applies an update to an event and returns it.
func (s *Store) UpdateEvent(ctx context.Context, householdID, id bson.ObjectID, update bson.M) (*Event, error) {
	return updateOne[Event](ctx, s.events, scoped(householdID, id), withUpdatedAt(update))
}

// DeleteEvent deletes an event.
func (s *Store) DeleteEvent(ctx context.Context, householdID, id bson.ObjectID) error {
	return deleteOne(ctx, s.events, scoped(householdID, id))
}

// withUpdatedAt adds "updatedAt = now" to the $set part of an update document.
func withUpdatedAt(update bson.M) bson.M {
	set, _ := update["$set"].(bson.M)
	if set == nil {
		set = bson.M{}
	}
	set["updatedAt"] = Now()
	update["$set"] = set
	return update
}
