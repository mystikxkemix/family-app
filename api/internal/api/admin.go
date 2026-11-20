package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"familyapp/internal/auth"
	"familyapp/internal/config"
	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var errAdminRequired = &apiError{Status: http.StatusForbidden, Code: "admin_required", Message: "only an admin can do this"}

// adminOnly is authed, plus the caller must be an admin.
func (s *Server) adminOnly(handler authedFunc) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, caller principal) error {
		if !caller.User.IsAdmin {
			return errAdminRequired
		}
		return handler(w, r, caller)
	})
}

// ensureAdmin makes sure the server has an admin. Databases created before the
// admin role existed get one: the ADMIN_USERNAME account, or the oldest one.
func (s *Server) ensureAdmin(ctx context.Context, admin AdminAccount) error {
	admins, err := s.store.CountAdmins(ctx)
	if err != nil || admins > 0 {
		return err
	}
	user, err := s.store.UserByUsername(ctx, strings.ToLower(strings.TrimSpace(admin.Username)))
	if errors.Is(err, store.ErrNotFound) {
		user, err = s.store.OldestUser(ctx)
	}
	if err != nil {
		return err
	}
	if _, err := s.store.UpdateUser(ctx, user.ID, bson.M{"isAdmin": true}); err != nil {
		return err
	}
	s.log.Warn("no admin yet: this account is now the admin", "username", user.Username)
	return nil
}

// --- Status -----------------------------------------------------------------------

type adminStatus struct {
	Version          string       `json:"version"`
	StartedAt        time.Time    `json:"startedAt"`
	Users            int          `json:"users"`
	Households       int          `json:"households"`
	PushEnabled      bool         `json:"pushEnabled"`
	Integrations     integrations `json:"integrations"`
	SettingsFile     string       `json:"settingsFile"`
	SettingsFound    bool         `json:"settingsFound"`
	SettingsWritable bool         `json:"settingsWritable"`
}

func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request, caller principal) error {
	users, err := s.store.AllUsers(r.Context())
	if err != nil {
		return err
	}
	households, err := s.store.AllHouseholds(r.Context())
	if err != nil {
		return err
	}
	_, statErr := os.Stat(s.options.SettingsFile)
	writeJSON(w, http.StatusOK, adminStatus{
		Version:          s.options.Version,
		StartedAt:        s.startedAt,
		Users:            len(users),
		Households:       len(households),
		PushEnabled:      s.notifier.Enabled(),
		Integrations:     s.integrations(),
		SettingsFile:     s.options.SettingsFile,
		SettingsFound:    statErr == nil,
		SettingsWritable: s.options.SettingsFile != "" && config.SettingsWritable(s.options.SettingsFile),
	})
	return nil
}

// --- Users ------------------------------------------------------------------------

type adminUser struct {
	store.User
	HouseholdName string `json:"householdName"`
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, caller principal) error {
	users, err := s.store.AllUsers(r.Context())
	if err != nil {
		return err
	}
	households, err := s.store.AllHouseholds(r.Context())
	if err != nil {
		return err
	}
	names := make(map[bson.ObjectID]string, len(households))
	for _, household := range households {
		names[household.ID] = household.Name
	}
	result := make([]adminUser, 0, len(users))
	for _, user := range users {
		result = append(result, adminUser{User: user, HouseholdName: names[user.HouseholdID]})
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

type adminUpdateUserRequest struct {
	IsAdmin *bool `json:"isAdmin"`
}

var errLastAdmin = &apiError{Status: http.StatusConflict, Code: "last_admin", Message: "the server needs at least one admin"}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request, caller principal) error {
	target, err := s.pathUser(r)
	if err != nil {
		return err
	}
	var request adminUpdateUserRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	if request.IsAdmin == nil {
		return badRequest("nothing to update")
	}
	if target.IsAdmin && !*request.IsAdmin {
		if err := s.keepOneAdmin(r.Context()); err != nil {
			return err
		}
	}
	updated, err := s.store.UpdateUser(r.Context(), target.ID, bson.M{"isAdmin": *request.IsAdmin})
	if err != nil {
		return err
	}
	s.log.Info("admin role changed", "by", caller.User.Username, "user", updated.Username, "isAdmin", updated.IsAdmin)
	writeJSON(w, http.StatusOK, updated)
	return nil
}

type resetPasswordResponse struct {
	TemporaryPassword string `json:"temporaryPassword"`
}

// adminResetPassword gives an account a random temporary password, ends its
// sessions, and makes it choose a new password at the next login.
func (s *Server) adminResetPassword(w http.ResponseWriter, r *http.Request, caller principal) error {
	target, err := s.pathUser(r)
	if err != nil {
		return err
	}
	temporary := auth.NewCode(3)
	hash, err := auth.HashPassword(temporary)
	if err != nil {
		return err
	}
	if _, err := s.store.UpdateUser(r.Context(), target.ID, bson.M{"passwordHash": hash, "mustChangePassword": true}); err != nil {
		return err
	}
	if err := s.store.DeleteUserSessions(r.Context(), target.ID); err != nil {
		return err
	}
	s.log.Info("password reset by an admin", "by", caller.User.Username, "user", target.Username)
	writeJSON(w, http.StatusOK, resetPasswordResponse{TemporaryPassword: temporary})
	return nil
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, caller principal) error {
	target, err := s.pathUser(r)
	if err != nil {
		return err
	}
	if target.ID == caller.User.ID {
		return &apiError{Status: http.StatusBadRequest, Code: "self", Message: "you cannot delete your own account"}
	}
	if target.IsAdmin {
		if err := s.keepOneAdmin(r.Context()); err != nil {
			return err
		}
	}
	if err := s.store.DeleteUser(r.Context(), target.ID); err != nil {
		return err
	}
	s.log.Info("account deleted by an admin", "by", caller.User.Username, "user", target.Username)
	noContent(w)
	return nil
}

func (s *Server) pathUser(r *http.Request) (*store.User, error) {
	id, err := pathID(r, "userId")
	if err != nil {
		return nil, err
	}
	return s.store.UserByID(r.Context(), id)
}

// keepOneAdmin refuses a change that would leave the server without an admin.
func (s *Server) keepOneAdmin(ctx context.Context) error {
	admins, err := s.store.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if admins <= 1 {
		return errLastAdmin
	}
	return nil
}

// --- Settings ---------------------------------------------------------------------

// The settings as the website sees them: secrets are never sent back, only
// whether they are set.
type adminArrSettings struct {
	URL              string `json:"url"`
	APIKey           string `json:"apiKey,omitempty"`
	APIKeySet        bool   `json:"apiKeySet"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolder       string `json:"rootFolder"`
}

type adminAPNsCredential struct {
	TeamID string   `json:"teamId"`
	KeyID  string   `json:"keyId"`
	Topics []string `json:"topics"`
	// Key is the content of the .p8 file (only sent by the website, "" keeps it).
	Key    string `json:"key,omitempty"`
	KeySet bool   `json:"keySet"`
}

type adminSettings struct {
	Radarr   adminArrSettings `json:"radarr"`
	Sonarr   adminArrSettings `json:"sonarr"`
	Prowlarr adminArrSettings `json:"prowlarr"`
	APNs     struct {
		Credentials []adminAPNsCredential `json:"credentials"`
	} `json:"apns"`
}

func maskedArr(settings config.ArrConfig) adminArrSettings {
	return adminArrSettings{
		URL:              settings.URL,
		APIKeySet:        settings.APIKey != "",
		QualityProfileID: settings.QualityProfileID,
		RootFolder:       settings.RootFolder,
	}
}

func masked(settings config.Settings) adminSettings {
	result := adminSettings{
		Radarr:   maskedArr(settings.Radarr),
		Sonarr:   maskedArr(settings.Sonarr),
		Prowlarr: maskedArr(settings.Prowlarr),
	}
	result.APNs.Credentials = []adminAPNsCredential{}
	for _, credential := range settings.APNs.Credentials {
		result.APNs.Credentials = append(result.APNs.Credentials, adminAPNsCredential{
			TeamID: credential.TeamID,
			KeyID:  credential.KeyID,
			Topics: credential.Topics,
			KeySet: credential.KeyFile != "",
		})
	}
	return result
}

func (s *Server) adminGetSettings(w http.ResponseWriter, r *http.Request, caller principal) error {
	writeJSON(w, http.StatusOK, masked(s.currentSettings()))
	return nil
}

var appleIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// adminPutSettings saves the settings file and applies it at once.
func (s *Server) adminPutSettings(w http.ResponseWriter, r *http.Request, caller principal) error {
	if s.options.SettingsFile == "" {
		return &apiError{Status: http.StatusConflict, Code: "not_writable", Message: "no settings file is configured (SETTINGS_FILE)"}
	}
	var request adminSettings
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	current := s.settings

	next := config.Settings{}
	var err error
	if next.Radarr, err = mergeArr("radarr", request.Radarr, current.Radarr); err != nil {
		return err
	}
	if next.Sonarr, err = mergeArr("sonarr", request.Sonarr, current.Sonarr); err != nil {
		return err
	}
	if next.Prowlarr, err = mergeArr("prowlarr", request.Prowlarr, current.Prowlarr); err != nil {
		return err
	}
	if next.APNs, err = s.mergeAPNs(request.APNs.Credentials, current.APNs); err != nil {
		return err
	}

	// Build the clients first: a bad key never reaches the file.
	sender, err := newSender(next.APNs)
	if err != nil {
		return badRequest("%v", err)
	}
	if err := config.SaveSettings(s.options.SettingsFile, next); err != nil {
		return settingsWriteError(err)
	}
	s.applySettingsLocked(next, sender)
	s.log.Info("settings saved from the website", "by", caller.User.Username)
	writeJSON(w, http.StatusOK, masked(next))
	return nil
}

// mergeArr validates one tool. An empty API key keeps the saved one; an empty
// URL turns the tool off.
func mergeArr(name string, request adminArrSettings, current config.ArrConfig) (config.ArrConfig, error) {
	address := strings.TrimRight(strings.TrimSpace(request.URL), "/")
	if address == "" {
		return config.ArrConfig{}, nil
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		return config.ArrConfig{}, badRequest("%s url must start with http:// or https://", name)
	}
	key := strings.TrimSpace(request.APIKey)
	if key == "" {
		key = current.APIKey
	}
	if key == "" {
		return config.ArrConfig{}, badRequest("%s needs an API key", name)
	}
	if request.QualityProfileID < 0 {
		return config.ArrConfig{}, badRequest("%s quality profile id is not valid", name)
	}
	return config.ArrConfig{
		URL:              address,
		APIKey:           key,
		QualityProfileID: request.QualityProfileID,
		RootFolder:       strings.TrimSpace(request.RootFolder),
	}, nil
}

// mergeAPNs validates the push keys and writes new .p8 files next to the
// settings file (apns/AuthKey_<keyId>.p8). An empty key keeps the saved file.
func (s *Server) mergeAPNs(requested []adminAPNsCredential, current push.Config) (push.Config, error) {
	saved := make(map[string]push.Credential, len(current.Credentials))
	for _, credential := range current.Credentials {
		saved[credential.KeyID] = credential
	}
	result := push.Config{Credentials: []push.Credential{}}
	for _, credential := range requested {
		teamID := strings.ToUpper(strings.TrimSpace(credential.TeamID))
		keyID := strings.ToUpper(strings.TrimSpace(credential.KeyID))
		if !appleIDPattern.MatchString(teamID) || !appleIDPattern.MatchString(keyID) {
			return push.Config{}, badRequest("APNs team id and key id are 10 letters or digits")
		}
		topics := []string{}
		for _, topic := range credential.Topics {
			if topic = strings.TrimSpace(topic); topic != "" {
				topics = append(topics, topic)
			}
		}
		if len(topics) == 0 {
			return push.Config{}, badRequest("APNs key %s needs at least one bundle id", keyID)
		}
		keyFile := saved[keyID].KeyFile
		if pemData := strings.TrimSpace(credential.Key); pemData != "" {
			if err := push.ValidateKey([]byte(pemData)); err != nil {
				return push.Config{}, badRequest("APNs key %s: %v", keyID, err)
			}
			keyFile = filepath.Join(filepath.Dir(s.options.SettingsFile), "apns", "AuthKey_"+keyID+".p8")
			if err := config.WriteFileAtomic(keyFile, []byte(pemData+"\n")); err != nil {
				return push.Config{}, settingsWriteError(err)
			}
		}
		if keyFile == "" {
			return push.Config{}, badRequest("APNs key %s: the .p8 file is missing", keyID)
		}
		result.Credentials = append(result.Credentials, push.Credential{TeamID: teamID, KeyID: keyID, KeyFile: keyFile, Topics: topics})
	}
	return result, nil
}

func settingsWriteError(err error) error {
	if errors.Is(err, config.ErrNotWritable) {
		return &apiError{
			Status:  http.StatusConflict,
			Code:    "not_writable",
			Message: "the API cannot write its data folder; on the server run: sudo chown -R 65532 /DATA/AppData/family-api/data (and mount the folder read-write)",
		}
	}
	return err
}

type testToolRequest struct {
	Tool string `json:"tool"`
}

type testToolResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// adminTestTool checks the saved address and key of one media tool.
func (s *Server) adminTestTool(w http.ResponseWriter, r *http.Request, caller principal) error {
	var request testToolRequest
	if err := decodeJSON(r, &request); err != nil {
		return err
	}
	services := s.arrServices()
	var ping func(context.Context) (string, error)
	switch request.Tool {
	case "radarr":
		if services.Radarr != nil {
			ping = services.Radarr.Ping
		}
	case "sonarr":
		if services.Sonarr != nil {
			ping = services.Sonarr.Ping
		}
	case "prowlarr":
		if services.Prowlarr != nil {
			ping = services.Prowlarr.Ping
		}
	default:
		return badRequest("tool must be radarr, sonarr or prowlarr")
	}
	if ping == nil {
		writeJSON(w, http.StatusOK, testToolResponse{OK: false, Message: request.Tool + " is not configured"})
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	version, err := ping(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, testToolResponse{OK: false, Message: err.Error()})
		return nil
	}
	writeJSON(w, http.StatusOK, testToolResponse{OK: true, Message: fmt.Sprintf("%s answered", version)})
	return nil
}

// newSender builds the APNs client, or nil when there is no key.
func newSender(settings push.Config) (push.Sender, error) {
	if len(settings.Credentials) == 0 {
		return nil, nil
	}
	return push.NewClient(settings)
}
