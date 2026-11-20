// Package api is the HTTP layer: routes, middlewares and handlers.
package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"familyapp/internal/auth"
	"familyapp/internal/push"
	"familyapp/internal/store"
)

// maxBodyBytes caps request bodies. The app only sends small JSON documents.
const maxBodyBytes = 64 << 10

// Options configures the server.
type Options struct {
	SessionTTL time.Duration
	// ClientIPHeader is the proxy header with the real client IP (see config).
	ClientIPHeader string
	Version        string
}

// Server holds the dependencies of the handlers.
type Server struct {
	store    *store.Store
	notifier *push.Notifier
	log      *slog.Logger
	options  Options

	setup *setupGate

	// requestLimiter limits every request per client IP.
	requestLimiter *auth.Limiter
	// loginLimiter limits login and signup attempts per client IP.
	loginLimiter *auth.Limiter
	// accountLimiter limits login attempts per username, whatever the IP.
	accountLimiter *auth.Limiter
}

// New builds the server. When the database has no user yet, it creates the
// one-time setup code that the first signup must give, and logs it.
func New(ctx context.Context, st *store.Store, notifier *push.Notifier, log *slog.Logger, options Options) (*Server, error) {
	s := &Server{
		store:          st,
		notifier:       notifier,
		log:            log,
		options:        options,
		setup:          &setupGate{},
		requestLimiter: auth.NewLimiter(600, 120),
		loginLimiter:   auth.NewLimiter(10, 10),
		accountLimiter: auth.NewLimiter(6, 10),
	}
	count, err := st.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		s.setup.code = auth.NewCode(3)
		log.Warn("no account yet: create the first one in the app with this setup code", "setupCode", s.setup.code)
	}
	return s, nil
}

// Handler returns the root HTTP handler with all routes and middlewares.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handle(s.health))

	// Public routes (rate limited per IP, see loginLimiter).
	mux.HandleFunc("POST /api/v1/auth/register", s.handle(s.register))
	mux.HandleFunc("POST /api/v1/auth/login", s.handle(s.login))

	// Account and household.
	mux.HandleFunc("POST /api/v1/auth/logout", s.authed(s.logout))
	mux.HandleFunc("GET /api/v1/me", s.authed(s.me))
	mux.HandleFunc("PATCH /api/v1/me", s.authed(s.updateMe))
	mux.HandleFunc("POST /api/v1/me/password", s.authed(s.changePassword))
	mux.HandleFunc("PATCH /api/v1/household", s.authed(s.updateHousehold))
	mux.HandleFunc("POST /api/v1/household/invites", s.authed(s.createInvite))
	mux.HandleFunc("PUT /api/v1/devices/{token}", s.authed(s.registerDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{token}", s.authed(s.unregisterDevice))

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

	// Events.
	mux.HandleFunc("GET /api/v1/events", s.authed(s.listEvents))
	mux.HandleFunc("POST /api/v1/events", s.authed(s.createEvent))
	mux.HandleFunc("PATCH /api/v1/events/{eventId}", s.authed(s.updateEvent))
	mux.HandleFunc("DELETE /api/v1/events/{eventId}", s.authed(s.deleteEvent))

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

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		header.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
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

// --- First signup -------------------------------------------------------------

// setupGate holds the one-time code for the very first account. Without it,
// anyone who finds the public URL first could create the first household.
type setupGate struct {
	mu   sync.Mutex
	code string
}

// check reports whether the given code is the current setup code.
func (g *setupGate) check(code string) bool {
	if g.code == "" {
		return false
	}
	expected := auth.NormalizeCode(g.code)
	return subtle.ConstantTimeCompare([]byte(auth.NormalizeCode(code)), []byte(expected)) == 1
}
