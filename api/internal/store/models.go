package store

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Household is a group of people who share everything in the app.
type Household struct {
	ID        bson.ObjectID `bson:"_id" json:"id"`
	Name      string        `bson:"name" json:"name"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
}

// User is an account. Every user belongs to exactly one household.
type User struct {
	ID           bson.ObjectID `bson:"_id" json:"id"`
	Username     string        `bson:"username" json:"username"`
	DisplayName  string        `bson:"displayName" json:"displayName"`
	PasswordHash string        `bson:"passwordHash" json:"-"`
	HouseholdID  bson.ObjectID `bson:"householdId" json:"-"`
	CreatedAt    time.Time     `bson:"createdAt" json:"createdAt"`
}

// Session is a login. Only the SHA-256 hash of the bearer token is stored.
type Session struct {
	ID          bson.ObjectID `bson:"_id"`
	TokenHash   string        `bson:"tokenHash"`
	UserID      bson.ObjectID `bson:"userId"`
	HouseholdID bson.ObjectID `bson:"householdId"`
	CreatedAt   time.Time     `bson:"createdAt"`
	RefreshedAt time.Time     `bson:"refreshedAt"`
	// ExpiresAt has a TTL index: MongoDB deletes the session after this date.
	ExpiresAt time.Time `bson:"expiresAt"`
}

// Invite is a one-time code that lets a new user join a household.
type Invite struct {
	ID          bson.ObjectID `bson:"_id"`
	CodeHash    string        `bson:"codeHash"`
	HouseholdID bson.ObjectID `bson:"householdId"`
	CreatedBy   bson.ObjectID `bson:"createdBy"`
	CreatedAt   time.Time     `bson:"createdAt"`
	ExpiresAt   time.Time     `bson:"expiresAt"`
	UsedAt      *time.Time    `bson:"usedAt,omitempty"`
}

// APNs environments a device token can belong to.
const (
	EnvironmentSandbox    = "sandbox"
	EnvironmentProduction = "production"
)

// Device is an iPhone that receives push notifications.
type Device struct {
	ID          bson.ObjectID `bson:"_id"`
	Token       string        `bson:"token"`
	UserID      bson.ObjectID `bson:"userId"`
	HouseholdID bson.ObjectID `bson:"householdId"`
	// Topic is the app bundle identifier. Each signing team uses its own one.
	Topic       string    `bson:"topic"`
	Environment string    `bson:"environment"`
	UpdatedAt   time.Time `bson:"updatedAt"`
}

// List kinds. They only change how the app shows a list.
const (
	ListKindGrocery = "grocery"
	ListKindTodo    = "todo"
	ListKindOther   = "other"
)

// List is a shared list: groceries, to-dos, packing list...
type List struct {
	ID          bson.ObjectID `bson:"_id" json:"id"`
	HouseholdID bson.ObjectID `bson:"householdId" json:"-"`
	Name        string        `bson:"name" json:"name"`
	Kind        string        `bson:"kind" json:"kind"`
	Emoji       string        `bson:"emoji" json:"emoji"`
	CreatedBy   bson.ObjectID `bson:"createdBy" json:"createdBy"`
	CreatedAt   time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt" json:"updatedAt"`
	// Counts are computed when listing, never stored.
	OpenCount  int64 `bson:"-" json:"openCount"`
	TotalCount int64 `bson:"-" json:"totalCount"`
}

// ListItem is one line of a list.
type ListItem struct {
	ID          bson.ObjectID  `bson:"_id" json:"id"`
	HouseholdID bson.ObjectID  `bson:"householdId" json:"-"`
	ListID      bson.ObjectID  `bson:"listId" json:"listId"`
	Text        string         `bson:"text" json:"text"`
	Note        string         `bson:"note" json:"note"`
	Checked     bool           `bson:"checked" json:"checked"`
	CheckedBy   *bson.ObjectID `bson:"checkedBy,omitempty" json:"checkedBy,omitempty"`
	CheckedAt   *time.Time     `bson:"checkedAt,omitempty" json:"checkedAt,omitempty"`
	CreatedBy   bson.ObjectID  `bson:"createdBy" json:"createdBy"`
	CreatedAt   time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// Media kinds.
const (
	MediaKindMovie = "movie"
	MediaKindShow  = "show"
)

// Media statuses, in watch order.
const (
	MediaStatusToWatch  = "toWatch"
	MediaStatusWatching = "watching"
	MediaStatusWatched  = "watched"
)

// ExternalIDs link a media to public databases. Sonarr uses TVDB, Radarr uses
// TMDB: keeping them lets the API send a media to those tools later.
type ExternalIDs struct {
	TMDB int    `bson:"tmdb,omitempty" json:"tmdb,omitempty"`
	TVDB int    `bson:"tvdb,omitempty" json:"tvdb,omitempty"`
	IMDB string `bson:"imdb,omitempty" json:"imdb,omitempty"`
}

// Media is a movie or a TV show on the shared watchlist.
type Media struct {
	ID          bson.ObjectID `bson:"_id" json:"id"`
	HouseholdID bson.ObjectID `bson:"householdId" json:"-"`
	Kind        string        `bson:"kind" json:"kind"`
	Title       string        `bson:"title" json:"title"`
	Year        int           `bson:"year,omitempty" json:"year,omitempty"`
	PosterURL   string        `bson:"posterUrl,omitempty" json:"posterUrl,omitempty"`
	Overview    string        `bson:"overview,omitempty" json:"overview,omitempty"`
	Note        string        `bson:"note" json:"note"`
	Status      string        `bson:"status" json:"status"`
	// InterestedUserIDs are the people who want to watch it.
	InterestedUserIDs []bson.ObjectID `bson:"interestedUserIds" json:"interestedUserIds"`
	ExternalIDs       ExternalIDs     `bson:"externalIds" json:"externalIds"`
	AddedBy           bson.ObjectID   `bson:"addedBy" json:"addedBy"`
	WatchedAt         *time.Time      `bson:"watchedAt,omitempty" json:"watchedAt,omitempty"`
	CreatedAt         time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time       `bson:"updatedAt" json:"updatedAt"`
}

// Event is a shared calendar entry.
type Event struct {
	ID          bson.ObjectID `bson:"_id" json:"id"`
	HouseholdID bson.ObjectID `bson:"householdId" json:"-"`
	Title       string        `bson:"title" json:"title"`
	StartsAt    time.Time     `bson:"startsAt" json:"startsAt"`
	EndsAt      *time.Time    `bson:"endsAt,omitempty" json:"endsAt,omitempty"`
	AllDay      bool          `bson:"allDay" json:"allDay"`
	Location    string        `bson:"location" json:"location"`
	Notes       string        `bson:"notes" json:"notes"`
	CreatedBy   bson.ObjectID `bson:"createdBy" json:"createdBy"`
	CreatedAt   time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt" json:"updatedAt"`
}
