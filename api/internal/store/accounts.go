package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// --- Households ---------------------------------------------------------------

// CreateHousehold inserts a household and fills its ID.
func (s *Store) CreateHousehold(ctx context.Context, household *Household) error {
	household.ID = bson.NewObjectID()
	household.CreatedAt = Now()
	return insertOne(ctx, s.households, household)
}

// HouseholdByID returns one household.
func (s *Store) HouseholdByID(ctx context.Context, id bson.ObjectID) (*Household, error) {
	return findOne[Household](ctx, s.households, bson.M{"_id": id})
}

// RenameHousehold changes the household name.
func (s *Store) RenameHousehold(ctx context.Context, id bson.ObjectID, name string) (*Household, error) {
	return updateOne[Household](ctx, s.households, bson.M{"_id": id}, bson.M{"$set": bson.M{"name": name}})
}

// --- Users --------------------------------------------------------------------

// CountUsers returns how many accounts exist.
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	return s.users.CountDocuments(ctx, bson.M{})
}

// CreateUser inserts a user. Returns ErrConflict when the username is taken.
func (s *Store) CreateUser(ctx context.Context, user *User) error {
	user.ID = bson.NewObjectID()
	user.CreatedAt = Now()
	return insertOne(ctx, s.users, user)
}

// UserByUsername returns the user with this (already normalized) username.
func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	return findOne[User](ctx, s.users, bson.M{"username": username})
}

// UserByID returns one user.
func (s *Store) UserByID(ctx context.Context, id bson.ObjectID) (*User, error) {
	return findOne[User](ctx, s.users, bson.M{"_id": id})
}

// UpdateUser sets fields on a user and returns it.
func (s *Store) UpdateUser(ctx context.Context, id bson.ObjectID, set bson.M) (*User, error) {
	return updateOne[User](ctx, s.users, bson.M{"_id": id}, bson.M{"$set": set})
}

// AllUsers returns every account of the server, oldest first.
func (s *Store) AllUsers(ctx context.Context) ([]User, error) {
	return findAll[User](ctx, s.users, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
}

// AllHouseholds returns every household of the server.
func (s *Store) AllHouseholds(ctx context.Context) ([]Household, error) {
	return findAll[Household](ctx, s.households, bson.M{})
}

// CountAdmins returns how many accounts are admins.
func (s *Store) CountAdmins(ctx context.Context) (int64, error) {
	return s.users.CountDocuments(ctx, bson.M{"isAdmin": true})
}

// OldestUser returns the first account created.
func (s *Store) OldestUser(ctx context.Context) (*User, error) {
	var user User
	opts := options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	err := s.users.FindOne(ctx, bson.M{}, opts).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// DeleteUser deletes an account with its sessions and push devices. What it
// added (lists, items...) stays: it belongs to the household.
func (s *Store) DeleteUser(ctx context.Context, id bson.ObjectID) error {
	if err := deleteOne(ctx, s.users, bson.M{"_id": id}); err != nil {
		return err
	}
	if _, err := s.sessions.DeleteMany(ctx, bson.M{"userId": id}); err != nil {
		return err
	}
	_, err := s.devices.DeleteMany(ctx, bson.M{"userId": id})
	return err
}

// DeleteUserSessions logs a user out everywhere.
func (s *Store) DeleteUserSessions(ctx context.Context, userID bson.ObjectID) error {
	_, err := s.sessions.DeleteMany(ctx, bson.M{"userId": userID})
	return err
}

// HouseholdMembers returns the users of a household, oldest first.
func (s *Store) HouseholdMembers(ctx context.Context, householdID bson.ObjectID) ([]User, error) {
	return findAll[User](ctx, s.users, bson.M{"householdId": householdID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
}

// --- Sessions -----------------------------------------------------------------

// CreateSession inserts a session and fills its ID and dates.
func (s *Store) CreateSession(ctx context.Context, session *Session, ttl time.Duration) error {
	now := Now()
	session.ID = bson.NewObjectID()
	session.CreatedAt = now
	session.RefreshedAt = now
	session.ExpiresAt = now.Add(ttl)
	return insertOne(ctx, s.sessions, session)
}

// SessionByTokenHash returns the live session for a token hash.
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	// The TTL monitor runs about once a minute, so also filter out expired ones.
	return findOne[Session](ctx, s.sessions, bson.M{"tokenHash": tokenHash, "expiresAt": bson.M{"$gt": Now()}})
}

// RefreshSession pushes the expiry date of a session that is still in use.
func (s *Store) RefreshSession(ctx context.Context, id bson.ObjectID, ttl time.Duration) error {
	now := Now()
	_, err := s.sessions.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"refreshedAt": now, "expiresAt": now.Add(ttl)}})
	return err
}

// DeleteSession removes one session (logout).
func (s *Store) DeleteSession(ctx context.Context, id bson.ObjectID) error {
	return deleteOne(ctx, s.sessions, bson.M{"_id": id})
}

// DeleteOtherSessions logs a user out everywhere except the given session.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keepID bson.ObjectID) error {
	_, err := s.sessions.DeleteMany(ctx, bson.M{"userId": userID, "_id": bson.M{"$ne": keepID}})
	return err
}

// --- Invites ------------------------------------------------------------------

// CreateInvite inserts an invite and fills its ID.
func (s *Store) CreateInvite(ctx context.Context, invite *Invite) error {
	invite.ID = bson.NewObjectID()
	invite.CreatedAt = Now()
	return insertOne(ctx, s.invites, invite)
}

// ConsumeInvite marks an unused, unexpired invite as used, in one atomic step,
// so the same code can never be used twice.
func (s *Store) ConsumeInvite(ctx context.Context, codeHash string) (*Invite, error) {
	now := Now()
	filter := bson.M{"codeHash": codeHash, "usedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": now}}
	return updateOne[Invite](ctx, s.invites, filter, bson.M{"$set": bson.M{"usedAt": now}})
}

// ReleaseInvite makes a consumed invite usable again (when the signup failed).
func (s *Store) ReleaseInvite(ctx context.Context, id bson.ObjectID) error {
	_, err := s.invites.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$unset": bson.M{"usedAt": ""}})
	return err
}

// --- Devices ------------------------------------------------------------------

// UpsertDevice saves a push token. A token moves to the new user if the phone
// changed account.
func (s *Store) UpsertDevice(ctx context.Context, device Device) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := s.devices.UpdateOne(ctx, bson.M{"token": device.Token}, bson.M{
		"$set": bson.M{
			"userId":      device.UserID,
			"householdId": device.HouseholdID,
			"topic":       device.Topic,
			"environment": device.Environment,
			"updatedAt":   Now(),
		},
		"$setOnInsert": bson.M{"_id": bson.NewObjectID()},
	}, opts)
	return err
}

// DeleteDevice removes a push token of a user (logout, notifications off).
func (s *Store) DeleteDevice(ctx context.Context, userID bson.ObjectID, token string) error {
	_, err := s.devices.DeleteOne(ctx, bson.M{"token": token, "userId": userID})
	return err
}

// DeleteDeviceByToken removes a push token that APNs reported as invalid.
func (s *Store) DeleteDeviceByToken(ctx context.Context, token string) error {
	_, err := s.devices.DeleteOne(ctx, bson.M{"token": token})
	return err
}

// DevicesForHousehold returns the devices of a household, minus one user's
// devices (the person who made the change does not need a notification).
func (s *Store) DevicesForHousehold(ctx context.Context, householdID, exceptUserID bson.ObjectID) ([]Device, error) {
	return findAll[Device](ctx, s.devices, bson.M{"householdId": householdID, "userId": bson.M{"$ne": exceptUserID}})
}
