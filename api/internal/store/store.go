// Package store keeps all data in MongoDB. Every read and write of shared data
// is scoped to a household, so one household never sees another one's data.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrNotFound means no document matched (or it belongs to another household).
	ErrNotFound = errors.New("not found")
	// ErrConflict means a unique value (username, token...) is already taken.
	ErrConflict = errors.New("already exists")
)

// Store wraps the MongoDB database and its collections.
type Store struct {
	client     *mongo.Client
	db         *mongo.Database
	households *mongo.Collection
	users      *mongo.Collection
	sessions   *mongo.Collection
	invites    *mongo.Collection
	devices    *mongo.Collection
	lists      *mongo.Collection
	listItems  *mongo.Collection
	media      *mongo.Collection
	places     *mongo.Collection
	events     *mongo.Collection
	// mediaDetails caches TMDB details (public data, shared by households).
	mediaDetails *mongo.Collection
}

// Open connects to MongoDB, checks the connection and creates the indexes.
func Open(ctx context.Context, uri, database string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(10 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("connect to MongoDB: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}
	db := client.Database(database)
	s := &Store{
		client:       client,
		db:           db,
		households:   db.Collection("households"),
		users:        db.Collection("users"),
		sessions:     db.Collection("sessions"),
		invites:      db.Collection("invites"),
		devices:      db.Collection("devices"),
		lists:        db.Collection("lists"),
		listItems:    db.Collection("list_items"),
		media:        db.Collection("media"),
		places:       db.Collection("places"),
		mediaDetails: db.Collection("media_details"),
		events:       db.Collection("events"),
	}
	if err := s.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return s, nil
}

// Close disconnects from MongoDB.
func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// Ping checks that MongoDB answers. Used by the health check.
func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

// DropDatabase deletes the whole database. Only tests use it.
func (s *Store) DropDatabase(ctx context.Context) error {
	return s.db.Drop(ctx)
}

func (s *Store) ensureIndexes(ctx context.Context) error {
	unique := func() *options.IndexOptionsBuilder { return options.Index().SetUnique(true) }
	// A TTL index with 0 seconds deletes each document at its own expiresAt date.
	expiring := func() *options.IndexOptionsBuilder { return options.Index().SetExpireAfterSeconds(0) }

	indexes := map[*mongo.Collection][]mongo.IndexModel{
		s.users: {
			{Keys: bson.D{{Key: "username", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "householdId", Value: 1}}},
		},
		s.sessions: {
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "userId", Value: 1}}},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: expiring()},
		},
		s.invites: {
			{Keys: bson.D{{Key: "codeHash", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: expiring()},
		},
		s.devices: {
			{Keys: bson.D{{Key: "token", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "householdId", Value: 1}}},
		},
		s.lists: {
			{Keys: bson.D{{Key: "householdId", Value: 1}, {Key: "createdAt", Value: 1}}},
		},
		s.listItems: {
			{Keys: bson.D{{Key: "householdId", Value: 1}, {Key: "listId", Value: 1}, {Key: "createdAt", Value: 1}}},
		},
		s.media: {
			{Keys: bson.D{{Key: "householdId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		},
		s.places: {
			{Keys: bson.D{{Key: "householdId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		},
		s.mediaDetails: {
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "tmdbId", Value: 1}, {Key: "lang", Value: 1}}, Options: unique()},
			// Details are fetched again after 30 days: ratings change.
			{Keys: bson.D{{Key: "fetchedAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(int32(DetailsCacheTTL.Seconds()))},
		},
		s.events: {
			{Keys: bson.D{{Key: "householdId", Value: 1}, {Key: "startsAt", Value: 1}}},
		},
	}
	for collection, models := range indexes {
		if _, err := collection.Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("create indexes on %s: %w", collection.Name(), err)
		}
	}
	return nil
}

// Now returns the current time in the precision MongoDB stores (milliseconds),
// so a value read back equals the value written.
func Now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// findAll runs a query and always returns a non-nil slice (JSON "[]", not "null").
func findAll[T any](ctx context.Context, collection *mongo.Collection, filter any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	cursor, err := collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	results := []T{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// findOne decodes one document, mapping "no document" to ErrNotFound.
func findOne[T any](ctx context.Context, collection *mongo.Collection, filter any) (*T, error) {
	var result T
	err := collection.FindOne(ctx, filter).Decode(&result)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// updateOne applies an update and returns the document after it.
func updateOne[T any](ctx context.Context, collection *mongo.Collection, filter, update any) (*T, error) {
	var result T
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// deleteOne deletes one document, returning ErrNotFound when nothing matched.
func deleteOne(ctx context.Context, collection *mongo.Collection, filter any) error {
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// insertOne inserts a document, mapping a unique index violation to ErrConflict.
func insertOne(ctx context.Context, collection *mongo.Collection, document any) error {
	_, err := collection.InsertOne(ctx, document)
	if mongo.IsDuplicateKeyError(err) {
		return ErrConflict
	}
	return err
}

// scoped is the filter for one document of a household.
func scoped(householdID, id bson.ObjectID) bson.M {
	return bson.M{"_id": id, "householdId": householdID}
}
