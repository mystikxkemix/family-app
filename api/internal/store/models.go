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
	// JellyfinUserID is the Jellyfin account of the household (one account
	// for the whole household). Empty: no Jellyfin sync.
	JellyfinUserID string `bson:"jellyfinUserId,omitempty" json:"-"`
	// JellyfinIgnoredItems are Jellyfin items whose watchlist title was
	// deleted: the sync never imports them again.
	JellyfinIgnoredItems []string `bson:"jellyfinIgnoredItems,omitempty" json:"-"`
}

// User is an account. Every user belongs to exactly one household.
type User struct {
	ID           bson.ObjectID `bson:"_id" json:"id"`
	Username     string        `bson:"username" json:"username"`
	DisplayName  string        `bson:"displayName" json:"displayName"`
	PasswordHash string        `bson:"passwordHash" json:"-"`
	HouseholdID  bson.ObjectID `bson:"householdId" json:"-"`
	// MustChangePassword blocks the account until the user picks a new
	// password. Set on the admin account created from environment variables.
	MustChangePassword bool `bson:"mustChangePassword" json:"mustChangePassword"`
	// IsAdmin opens the admin section of the website (users, settings).
	IsAdmin   bool      `bson:"isAdmin" json:"isAdmin"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
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
	// MatchCheckedAt is when the automatic TMDB matching looked at this title
	// (matched or not), so it is not searched again and again.
	MatchCheckedAt *time.Time `bson:"matchCheckedAt,omitempty" json:"-"`
	// DownloadRequestedAt is when someone sent it to Radarr or Sonarr.
	DownloadRequestedAt *time.Time `bson:"downloadRequestedAt,omitempty" json:"downloadRequestedAt,omitempty"`
	// Source is "jellyfin" for a title the Jellyfin sync added, "" otherwise.
	Source string `bson:"source,omitempty" json:"source,omitempty"`
	// StatusSetBy is who set the status last: "user" or "jellyfin". A status
	// set by a user is kept until Jellyfin's played state changes.
	StatusSetBy string `bson:"statusSetBy,omitempty" json:"-"`
	// Jellyfin is the matching Jellyfin item, when there is one.
	Jellyfin  *MediaJellyfin `bson:"jellyfin,omitempty" json:"jellyfin,omitempty"`
	CreatedAt time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// Who changed a watchlist status last (Media.StatusSetBy).
const (
	StatusSetByUser     = "user"
	StatusSetByJellyfin = "jellyfin"
	// SourceJellyfin marks the titles the Jellyfin sync added.
	SourceJellyfin = "jellyfin"
)

// Played states of a Jellyfin item (MediaJellyfin.State).
const (
	JellyfinStateNone     = ""
	JellyfinStateWatching = "watching"
	JellyfinStateWatched  = "watched"
)

// MediaJellyfin links a watchlist title to a Jellyfin item.
type MediaJellyfin struct {
	ItemID string `bson:"itemId" json:"-"`
	// Available is false once the item is no longer in Jellyfin.
	Available bool `bson:"available" json:"available"`
	// PlayedAt is the last time it was played in Jellyfin.
	PlayedAt *time.Time `bson:"playedAt,omitempty" json:"playedAt,omitempty"`
	// State is the played state seen at the last sync (JellyfinState*). A
	// change of state is what lets the sync move a status set by a user.
	State    string    `bson:"state" json:"-"`
	SyncedAt time.Time `bson:"syncedAt" json:"-"`
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

// Place categories.
const (
	PlaceCategoryRestaurant = "restaurant"
	PlaceCategoryBar        = "bar"
	PlaceCategoryCafe       = "cafe"
	PlaceCategoryActivity   = "activity"
	PlaceCategoryShop       = "shop"
	PlaceCategoryOther      = "other"
)

// PlaceCategories lists the allowed categories, for validation.
var PlaceCategories = []string{
	PlaceCategoryRestaurant, PlaceCategoryBar, PlaceCategoryCafe,
	PlaceCategoryActivity, PlaceCategoryShop, PlaceCategoryOther,
}

// Place statuses.
const (
	PlaceStatusToTry = "toTry"
	PlaceStatusTried = "tried"
)

// Place is a restaurant, bar or activity the household wants to try.
type Place struct {
	ID          bson.ObjectID `bson:"_id" json:"id"`
	HouseholdID bson.ObjectID `bson:"householdId" json:"-"`
	Name        string        `bson:"name" json:"name"`
	Category    string        `bson:"category" json:"category"`
	Address     string        `bson:"address" json:"address"`
	URL         string        `bson:"url" json:"url"`
	Note        string        `bson:"note" json:"note"`
	Status      string        `bson:"status" json:"status"`
	// Rating is 1 to 5 once tried, 0 when not rated.
	Rating            int             `bson:"rating" json:"rating"`
	InterestedUserIDs []bson.ObjectID `bson:"interestedUserIds" json:"interestedUserIds"`
	AddedBy           bson.ObjectID   `bson:"addedBy" json:"addedBy"`
	TriedAt           *time.Time      `bson:"triedAt,omitempty" json:"triedAt,omitempty"`
	CreatedAt         time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time       `bson:"updatedAt" json:"updatedAt"`
}
