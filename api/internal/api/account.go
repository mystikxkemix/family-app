package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"familyapp/internal/auth"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 256
	inviteLifetime    = 7 * 24 * time.Hour
	// sessionRefreshEvery limits session writes to one a day per session.
	sessionRefreshEvery = 24 * time.Hour
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

// principal is the logged-in caller of a request.
type principal struct {
	SessionID   bson.ObjectID
	User        store.User
	HouseholdID bson.ObjectID
}

// authedFunc is a handler that runs only for a logged-in caller.
type authedFunc func(w http.ResponseWriter, r *http.Request, caller principal) error

// authed checks the "Authorization: Bearer <token>" header before the handler.
func (s *Server) authed(handler authedFunc) http.HandlerFunc {
	return s.handle(func(w http.ResponseWriter, r *http.Request) error {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || token == "" {
			return errUnauthorized
		}
		session, err := s.store.SessionByTokenHash(r.Context(), auth.HashToken(token))
		if errors.Is(err, store.ErrNotFound) {
			return errUnauthorized
		}
		if err != nil {
			return err
		}
		user, err := s.store.UserByID(r.Context(), session.UserID)
		if errors.Is(err, store.ErrNotFound) {
			return errUnauthorized
		}
		if err != nil {
			return err
		}
		// Sliding expiry: a session used at least once per SessionTTL never expires.
		if time.Since(session.RefreshedAt) > sessionRefreshEvery {
			if err := s.store.RefreshSession(r.Context(), session.ID, s.options.SessionTTL); err != nil {
				s.log.Warn("refresh session", "error", err)
			}
		}
		return handler(w, r, principal{SessionID: session.ID, User: *user, HouseholdID: user.HouseholdID})
	})
}

// --- Signup and login ---------------------------------------------------------

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	// SetupCode creates the first account and its household.
	SetupCode string `json:"setupCode"`
	// InviteCode joins the household that created the invite.
	InviteCode string `json:"inviteCode"`
	// HouseholdName names the new household (setup code only).
	HouseholdName string `json:"householdName"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  store.User `json:"user"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	if !s.loginLimiter.Allow(s.clientIP(r)) {
		return errRateLimited
	}
	var request registerRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	username, err := normalizeUsername(request.Username)
	if err != nil {
		return err
	}
	if err := checkPassword(request.Password); err != nil {
		return err
	}
	displayName, err := cleanText("displayName", request.DisplayName, true, 40)
	if err != nil {
		return err
	}
	user := &store.User{Username: username, DisplayName: displayName}
	if user.PasswordHash, err = auth.HashPassword(request.Password); err != nil {
		return err
	}

	switch {
	case request.InviteCode != "":
		err = s.registerWithInvite(r.Context(), user, request.InviteCode)
	case request.SetupCode != "":
		err = s.registerFirstUser(r.Context(), user, request.SetupCode, request.HouseholdName)
	default:
		err = badRequest("an invite code is required")
	}
	if err != nil {
		return err
	}
	return s.startSession(w, r.Context(), user, http.StatusCreated)
}

// registerFirstUser creates the first household and its first member.
func (s *Server) registerFirstUser(ctx context.Context, user *store.User, setupCode, householdName string) error {
	s.setup.mu.Lock()
	defer s.setup.mu.Unlock()
	if !s.setup.check(setupCode) {
		return &apiError{Status: http.StatusForbidden, Code: "invalid_code", Message: "invalid or expired code"}
	}
	name, err := cleanText("householdName", householdName, false, 40)
	if err != nil {
		return err
	}
	if name == "" {
		name = "Home"
	}
	household := &store.Household{Name: name}
	if err := s.store.CreateHousehold(ctx, household); err != nil {
		return err
	}
	user.HouseholdID = household.ID
	if err := s.store.CreateUser(ctx, user); err != nil {
		return err
	}
	s.setup.code = ""
	s.log.Info("first account created; the setup code is no longer valid")
	return nil
}

// registerWithInvite adds a member to the household of the invite.
func (s *Server) registerWithInvite(ctx context.Context, user *store.User, inviteCode string) error {
	// Check the username first, so a taken name does not burn the invite.
	if _, err := s.store.UserByUsername(ctx, user.Username); err == nil {
		return store.ErrConflict
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	invite, err := s.store.ConsumeInvite(ctx, auth.HashToken(auth.NormalizeCode(inviteCode)))
	if errors.Is(err, store.ErrNotFound) {
		return &apiError{Status: http.StatusForbidden, Code: "invalid_code", Message: "invalid or expired code"}
	}
	if err != nil {
		return err
	}
	user.HouseholdID = invite.HouseholdID
	if err := s.store.CreateUser(ctx, user); err != nil {
		if releaseErr := s.store.ReleaseInvite(ctx, invite.ID); releaseErr != nil {
			s.log.Error("release invite", "error", releaseErr)
		}
		return err
	}
	return nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	if !s.loginLimiter.Allow(s.clientIP(r)) {
		return errRateLimited
	}
	var request loginRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	invalid := &apiError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "wrong username or password"}
	username := strings.ToLower(strings.TrimSpace(request.Username))
	if len(request.Password) > maxPasswordLength || len(username) > 64 {
		return invalid
	}
	if !s.accountLimiter.Allow(username) {
		return errRateLimited
	}

	user, err := s.store.UserByUsername(r.Context(), username)
	if errors.Is(err, store.ErrNotFound) {
		auth.BurnPasswordCheck(request.Password)
		return invalid
	}
	if err != nil {
		return err
	}
	ok, err := auth.CheckPassword(request.Password, user.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		return invalid
	}
	return s.startSession(w, r.Context(), user, http.StatusOK)
}

// startSession creates a session and answers with its token. The token is
// shown once and only its hash is stored.
func (s *Server) startSession(w http.ResponseWriter, ctx context.Context, user *store.User, status int) error {
	token := auth.NewToken()
	session := &store.Session{TokenHash: auth.HashToken(token), UserID: user.ID, HouseholdID: user.HouseholdID}
	if err := s.store.CreateSession(ctx, session, s.options.SessionTTL); err != nil {
		return err
	}
	writeJSON(w, status, authResponse{Token: token, User: *user})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, caller principal) error {
	if err := s.store.DeleteSession(r.Context(), caller.SessionID); err != nil {
		return err
	}
	noContent(w)
	return nil
}

// --- Profile and household ----------------------------------------------------

type meResponse struct {
	User      store.User      `json:"user"`
	Household store.Household `json:"household"`
	Members   []store.User    `json:"members"`
	// PushEnabled tells the app whether the server can send notifications.
	PushEnabled bool `json:"pushEnabled"`
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, caller principal) error {
	household, err := s.store.HouseholdByID(r.Context(), caller.HouseholdID)
	if err != nil {
		return err
	}
	members, err := s.store.HouseholdMembers(r.Context(), caller.HouseholdID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, meResponse{User: caller.User, Household: *household, Members: members, PushEnabled: s.notifier.Enabled()})
	return nil
}

type updateMeRequest struct {
	DisplayName *string `json:"displayName"`
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request updateMeRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	user := &caller.User
	if request.DisplayName != nil {
		name, err := cleanText("displayName", *request.DisplayName, true, 40)
		if err != nil {
			return err
		}
		if user, err = s.store.UpdateUser(r.Context(), caller.User.ID, bson.M{"displayName": name}); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, user)
	return nil
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// changePassword also logs the user out of every other device.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request changePasswordRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if !s.accountLimiter.Allow(caller.User.Username) {
		return errRateLimited
	}
	ok, err := auth.CheckPassword(request.CurrentPassword, caller.User.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		return &apiError{Status: http.StatusForbidden, Code: "invalid_credentials", Message: "the current password is wrong"}
	}
	if err := checkPassword(request.NewPassword); err != nil {
		return err
	}
	hash, err := auth.HashPassword(request.NewPassword)
	if err != nil {
		return err
	}
	if _, err := s.store.UpdateUser(r.Context(), caller.User.ID, bson.M{"passwordHash": hash}); err != nil {
		return err
	}
	if err := s.store.DeleteOtherSessions(r.Context(), caller.User.ID, caller.SessionID); err != nil {
		return err
	}
	noContent(w)
	return nil
}

type updateHouseholdRequest struct {
	Name *string `json:"name"`
}

func (s *Server) updateHousehold(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request updateHouseholdRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if request.Name == nil {
		return badRequest("nothing to update")
	}
	name, err := cleanText("name", *request.Name, true, 40)
	if err != nil {
		return err
	}
	household, err := s.store.RenameHousehold(r.Context(), caller.HouseholdID, name)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, household)
	return nil
}

type inviteResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// createInvite returns a one-time code, valid 7 days, to add a member.
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, caller principal) error {
	code := auth.NewCode(3)
	invite := &store.Invite{
		CodeHash:    auth.HashToken(auth.NormalizeCode(code)),
		HouseholdID: caller.HouseholdID,
		CreatedBy:   caller.User.ID,
		ExpiresAt:   store.Now().Add(inviteLifetime),
	}
	if err := s.store.CreateInvite(r.Context(), invite); err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, inviteResponse{Code: code, ExpiresAt: invite.ExpiresAt})
	return nil
}

// --- Push devices -------------------------------------------------------------

var deviceTokenPattern = regexp.MustCompile(`^[0-9a-fA-F]{32,200}$`)

type registerDeviceRequest struct {
	// Topic is the app bundle identifier.
	Topic       string `json:"topic"`
	Environment string `json:"environment"`
}

type registerDeviceResponse struct {
	// Supported is false when the server has no APNs key for this app build.
	Supported bool `json:"supported"`
}

func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request, caller principal) error {
	token := r.PathValue("token")
	if !deviceTokenPattern.MatchString(token) {
		return badRequest("invalid device token")
	}
	var request registerDeviceRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	topic, err := cleanText("topic", request.Topic, true, 200)
	if err != nil {
		return err
	}
	if err := oneOf("environment", request.Environment, store.EnvironmentSandbox, store.EnvironmentProduction); err != nil {
		return err
	}
	err = s.store.UpsertDevice(r.Context(), store.Device{
		Token:       strings.ToLower(token),
		UserID:      caller.User.ID,
		HouseholdID: caller.HouseholdID,
		Topic:       topic,
		Environment: request.Environment,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, registerDeviceResponse{Supported: s.notifier.Supports(topic)})
	return nil
}

func (s *Server) unregisterDevice(w http.ResponseWriter, r *http.Request, caller principal) error {
	if err := s.store.DeleteDevice(r.Context(), caller.User.ID, strings.ToLower(r.PathValue("token"))); err != nil {
		return err
	}
	noContent(w)
	return nil
}

// --- Validation ---------------------------------------------------------------

func normalizeUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return "", badRequest("username must be 3 to 32 characters: letters, digits, dot, dash or underscore")
	}
	return username, nil
}

func checkPassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLength || len(password) > maxPasswordLength {
		return badRequest("password must be at least %d characters", minPasswordLength)
	}
	return nil
}
