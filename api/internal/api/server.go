// Package api is the HTTP layer: routes, middlewares and handlers.
package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"familyapp/internal/arr"
	"familyapp/internal/auth"
	"familyapp/internal/config"
	"familyapp/internal/jellyfin"
	"familyapp/internal/push"
	"familyapp/internal/store"
	"familyapp/internal/tmdb"
)

// maxBodyBytes caps request bodies. The app only sends small JSON documents.
const maxBodyBytes = 64 << 10

// Options configures the server.
type Options struct {
	SessionTTL time.Duration
	// ClientIPHeader is the proxy header with the real client IP (see config).
	ClientIPHeader string
	Version        string
	// SettingsFile is where the admin page saves the settings ("" = read only).
	SettingsFile string
	// Settings are the APNs keys and media tools loaded at start.
	Settings config.Settings
	// Admin is the first account, created when the database has no user.
	Admin AdminAccount
	// TMDBBaseURL replaces the TMDB API address (tests only).
	TMDBBaseURL string
}

// AdminAccount is the first account, from the ADMIN_* environment variables.
type AdminAccount struct {
	Username string
	Password string
}

// Server holds the dependencies of the handlers.
type Server struct {
	store    *store.Store
	notifier *push.Notifier
	log      *slog.Logger
	options  Options
	// startedAt is shown on the admin status page.
	startedAt time.Time

	// settingsMu guards settings and the clients built from them, which the
	// admin can change at run time.
	settingsMu sync.Mutex
	settings   config.Settings
	arr        atomic.Pointer[arr.Services]
	// tmdb is nil when no TMDB key is set.
	tmdb atomic.Pointer[tmdb.Client]
	// jellyfin is nil when Jellyfin is not set up.
	jellyfin atomic.Pointer[jellyfin.Client]

	// Automatic TMDB matching (details.go).
	matchTrigger chan struct{}
	matchStop    chan struct{}
	matchDone    chan struct{}

	// Jellyfin sync (jellyfin.go). jellyfinRunning is true while a sync runs.
	jellyfinTrigger  chan struct{}
	jellyfinStop     chan struct{}
	jellyfinDone     chan struct{}
	jellyfinRunning  atomic.Bool
	lastJellyfinSync atomic.Pointer[jellyfinSyncResult]

	// requestLimiter limits every request per client IP.
	requestLimiter *auth.Limiter
	// loginLimiter limits login and signup attempts per client IP.
	loginLimiter *auth.Limiter
	// accountLimiter limits login attempts per username, whatever the IP.
	accountLimiter *auth.Limiter
}

// New builds the server. When the database has no user yet, it creates the
// admin account (see createAdmin).
func New(ctx context.Context, st *store.Store, notifier *push.Notifier, log *slog.Logger, options Options) (*Server, error) {
	s := &Server{
		store:          st,
		notifier:       notifier,
		log:            log,
		options:        options,
		startedAt:      time.Now().UTC(),
		requestLimiter: auth.NewLimiter(600, 120),
		loginLimiter:   auth.NewLimiter(10, 10),
		accountLimiter: auth.NewLimiter(6, 10),
	}
	sender, err := newSender(options.Settings.APNs)
	if err != nil {
		return nil, err
	}
	s.settingsMu.Lock()
	s.applySettingsLocked(options.Settings, sender)
	s.settingsMu.Unlock()

	count, err := st.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		if err := s.createAdmin(ctx, options.Admin); err != nil {
			return nil, err
		}
	}
	if err := s.ensureAdmin(ctx, options.Admin); err != nil {
		return nil, err
	}
	s.startMatcher()
	s.startJellyfinSync()
	return s, nil
}

// applySettingsLocked switches to new settings: media tool clients and the
// APNs client. The caller holds settingsMu.
func (s *Server) applySettingsLocked(settings config.Settings, sender push.Sender) {
	keyChanged := settings.TMDB.APIKey != s.settings.TMDB.APIKey
	jellyfinChanged := settings.Jellyfin != s.settings.Jellyfin
	s.settings = settings
	s.arr.Store(arr.New(settings.Radarr, settings.Sonarr, settings.Prowlarr))
	s.notifier.SetSender(sender)
	if settings.Jellyfin.Enabled() {
		s.jellyfin.Store(jellyfin.New(settings.Jellyfin.URL, settings.Jellyfin.APIKey))
	} else {
		s.jellyfin.Store(nil)
	}
	if jellyfinChanged {
		s.triggerJellyfin()
	}
	if !settings.TMDB.Enabled() {
		s.tmdb.Store(nil)
		return
	}
	client := tmdb.New(settings.TMDB.APIKey)
	if s.options.TMDBBaseURL != "" {
		client.WithBaseURL(s.options.TMDBBaseURL)
	}
	s.tmdb.Store(client)
	if keyChanged {
		// A new key: try again to link the titles without details.
		s.triggerMatch()
	}
}

// currentSettings returns a copy of the settings in use.
func (s *Server) currentSettings() config.Settings {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.settings
}

// arrServices returns the media tool clients in use.
func (s *Server) arrServices() *arr.Services {
	return s.arr.Load()
}

// Handler returns the root HTTP handler with all routes and middlewares.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handle(s.health))

	// Public routes (rate limited per IP, see loginLimiter).
	mux.HandleFunc("POST /api/v1/auth/register", s.handle(s.register))
	mux.HandleFunc("POST /api/v1/auth/login", s.handle(s.login))

	// Account and household.
	// Open to an account that must still change its password.
	mux.HandleFunc("POST /api/v1/auth/logout", s.authedDuringPasswordChange(s.logout))
	mux.HandleFunc("GET /api/v1/me", s.authedDuringPasswordChange(s.me))
	mux.HandleFunc("POST /api/v1/me/password", s.authedDuringPasswordChange(s.changePassword))
	mux.HandleFunc("PUT /api/v1/devices/{token}", s.authedDuringPasswordChange(s.registerDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{token}", s.authedDuringPasswordChange(s.unregisterDevice))
	mux.HandleFunc("PATCH /api/v1/me", s.authed(s.updateMe))
	mux.HandleFunc("PATCH /api/v1/household", s.authed(s.updateHousehold))
	mux.HandleFunc("POST /api/v1/household/invites", s.authed(s.createInvite))

	// Shared lists.
	mux.HandleFunc("GET /api/v1/lists", s.authed(s.listLists))
	mux.HandleFunc("POST /api/v1/lists", s.authed(s.createList))
	mux.HandleFunc("PATCH /api/v1/lists/{listId}", s.authed(s.updateList))
	mux.HandleFunc("DELETE /api/v1/lists/{listId}", s.authed(s.deleteList))
	mux.HandleFunc("GET /api/v1/lists/{listId}/items", s.authed(s.listItems))
	mux.HandleFunc("POST /api/v1/lists/{listId}/items", s.authed(s.createListItem))
	mux.HandleFunc("PATCH /api/v1/lists/{listId}/items/{itemId}", s.authed(s.updateListItem))
	mux.HandleFunc("DELETE /api/v1/lists/{listId}/items/{itemId}", s.authed(s.deleteListItem))
	mux.HandleFunc("POST /api/v1/lists/{listId}/clear-checked", s.authed(s.clearCheckedItems))

	// Watchlist.
	mux.HandleFunc("GET /api/v1/watchlist", s.authed(s.listMedia))
	mux.HandleFunc("POST /api/v1/watchlist", s.authed(s.createMedia))
	mux.HandleFunc("PATCH /api/v1/watchlist/{mediaId}", s.authed(s.updateMedia))
	mux.HandleFunc("DELETE /api/v1/watchlist/{mediaId}", s.authed(s.deleteMedia))
	mux.HandleFunc("POST /api/v1/watchlist/{mediaId}/download", s.authed(s.downloadMedia))
	mux.HandleFunc("GET /api/v1/watchlist/{mediaId}/details", s.authed(s.mediaDetails))
	mux.HandleFunc("POST /api/v1/watchlist/{mediaId}/match", s.authed(s.matchMedia))

	// Media tools (Radarr, Sonarr, Prowlarr), when configured.
	mux.HandleFunc("GET /api/v1/search/media", s.authed(s.searchMedia))
	mux.HandleFunc("GET /api/v1/releases", s.authed(s.searchReleases))
	mux.HandleFunc("POST /api/v1/releases/grab", s.authed(s.grabRelease))

	// Places to try.
	mux.HandleFunc("GET /api/v1/places", s.authed(s.listPlaces))
	mux.HandleFunc("POST /api/v1/places", s.authed(s.createPlace))
	mux.HandleFunc("PATCH /api/v1/places/{placeId}", s.authed(s.updatePlace))
	mux.HandleFunc("DELETE /api/v1/places/{placeId}", s.authed(s.deletePlace))

	// Events.
	mux.HandleFunc("GET /api/v1/events", s.authed(s.listEvents))
	mux.HandleFunc("POST /api/v1/events", s.authed(s.createEvent))
	mux.HandleFunc("PATCH /api/v1/events/{eventId}", s.authed(s.updateEvent))
	mux.HandleFunc("DELETE /api/v1/events/{eventId}", s.authed(s.deleteEvent))

	// Admin section of the website.
	mux.HandleFunc("GET /api/v1/admin/status", s.adminOnly(s.adminStatus))
	mux.HandleFunc("GET /api/v1/admin/users", s.adminOnly(s.adminUsers))
	mux.HandleFunc("PATCH /api/v1/admin/users/{userId}", s.adminOnly(s.adminUpdateUser))
	mux.HandleFunc("POST /api/v1/admin/users/{userId}/reset-password", s.adminOnly(s.adminResetPassword))
	mux.HandleFunc("DELETE /api/v1/admin/users/{userId}", s.adminOnly(s.adminDeleteUser))
	mux.HandleFunc("GET /api/v1/admin/settings", s.adminOnly(s.adminGetSettings))
	mux.HandleFunc("PUT /api/v1/admin/settings", s.adminOnly(s.adminPutSettings))
	mux.HandleFunc("POST /api/v1/admin/settings/test", s.adminOnly(s.adminTestTool))
	mux.HandleFunc("GET /api/v1/admin/connections", s.adminOnly(s.adminConnections))
	mux.HandleFunc("GET /api/v1/admin/jellyfin", s.adminOnly(s.adminJellyfin))
	mux.HandleFunc("GET /api/v1/admin/jellyfin/users", s.adminOnly(s.adminJellyfinUsers))
	mux.HandleFunc("PUT /api/v1/admin/jellyfin/household", s.adminOnly(s.adminSetJellyfinUser))
	mux.HandleFunc("POST /api/v1/admin/jellyfin/sync", s.adminOnly(s.adminSyncJellyfin))

	// Unknown API paths answer JSON; everything else is the website.
	mux.HandleFunc("/api/", s.handle(func(w http.ResponseWriter, r *http.Request) error { return errNotFound }))
	mux.Handle("/", s.website())

	return s.recoverPanics(s.securityHeaders(s.limitRequests(s.logRequests(mux))))
}

// handle turns a handlerFunc into an http.HandlerFunc that writes its error.
func (s *Server) handle(handler handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := handler(w, r); err != nil {
			if internal := writeError(w, err); internal {
				s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
			}
		}
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.Error("health check: database unreachable", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unreachable"})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.options.Version})
	return nil
}

// clientIP returns the caller's IP, from the trusted proxy header if set.
func (s *Server) clientIP(r *http.Request) string {
	if s.options.ClientIPHeader != "" {
		if value := r.Header.Get(s.options.ClientIPHeader); value != "" {
			// X-Forwarded-For is a list; the last entry is the one our own proxy
			// added, the others come from the client and can be faked.
			parts := strings.Split(value, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- Middlewares --------------------------------------------------------------

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				s.log.Error("panic", "path", r.URL.Path, "panic", recovered, "stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error", "code": "internal"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Content-Security-Policy of the website: only its own files, images from
// https (posters), and API calls to itself.
const websiteCSP = "default-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		if isAPIPath(r.URL.Path) {
			header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			header.Set("Cache-Control", "no-store")
		} else {
			header.Set("Content-Security-Policy", websiteCSP)
		}
		next.ServeHTTP(w, r)
	})
}

func isAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/healthz"
}

func (s *Server) limitRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requestLimiter.Allow(s.clientIP(r)) {
			writeError(w, errRateLimited)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		if r.URL.Path == "/healthz" {
			return
		}
		// The path never holds secrets (device tokens are not secret), and the
		// query string and headers are not logged.
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", recorder.status,
			"duration", time.Since(start).Round(time.Millisecond), "ip", s.clientIP(r))
	})
}
