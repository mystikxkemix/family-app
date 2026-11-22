package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"familyapp/internal/arr"
	"familyapp/internal/jellyfin"
	"familyapp/internal/push"
	"familyapp/internal/tmdb"
)

// connectionTimeout limits each test, so one slow service never blocks the page.
const connectionTimeout = 10 * time.Second

// connectionResult is one row of Settings > Connections.
type connectionResult struct {
	Service    string `json:"service"`
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	// Message is the version on success, the reason on failure. It never
	// contains a key, but can contain an internal address: admins only.
	Message   string `json:"message"`
	LatencyMS int64  `json:"latencyMs"`
}

// connectionCheck tests one service. A nil test means "not set up".
type connectionCheck struct {
	service string
	test    func(context.Context) (string, error)
}

// adminConnections tests every outside service the API uses, in parallel.
func (s *Server) adminConnections(w http.ResponseWriter, r *http.Request, caller principal) error {
	checks := s.connectionChecks()
	results := make([]connectionResult, len(checks))
	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runCheck(r.Context(), check)
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, results)
	return nil
}

func runCheck(parent context.Context, check connectionCheck) connectionResult {
	result := connectionResult{Service: check.service}
	if check.test == nil {
		result.Message = "not set up"
		return result
	}
	result.Configured = true
	ctx, cancel := context.WithTimeout(parent, connectionTimeout)
	defer cancel()
	start := time.Now()
	message, err := check.test(ctx)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		// "media tool error: Prowlarr is not reachable..." → the reason only.
		message := err.Error()
		for _, prefix := range []string{arr.ErrUpstream.Error() + ": ", tmdb.ErrUpstream.Error() + ": ", jellyfin.ErrUpstream.Error() + ": "} {
			message = strings.TrimPrefix(message, prefix)
		}
		result.Message = message
		return result
	}
	result.OK = true
	result.Message = message
	return result
}

// connectionChecks lists the services in a fixed order (the page order).
func (s *Server) connectionChecks() []connectionCheck {
	services := s.arrServices()
	checks := []connectionCheck{{
		service: "database",
		test: func(ctx context.Context) (string, error) {
			if err := s.store.Ping(ctx); err != nil {
				return "", fmt.Errorf("MongoDB is not reachable: %v", err)
			}
			return "MongoDB", nil
		},
	}}
	radarr := connectionCheck{service: "radarr"}
	if services.Radarr != nil {
		radarr.test = services.Radarr.Ping
	}
	sonarr := connectionCheck{service: "sonarr"}
	if services.Sonarr != nil {
		sonarr.test = services.Sonarr.Ping
	}
	prowlarr := connectionCheck{service: "prowlarr"}
	if services.Prowlarr != nil {
		prowlarr.test = services.Prowlarr.Ping
	}
	tmdbCheck := connectionCheck{service: "tmdb"}
	if client := s.tmdbClient(); client != nil {
		tmdbCheck.test = client.Ping
	}
	jellyfinCheck := connectionCheck{service: "jellyfin"}
	if client := s.jellyfinClient(); client != nil {
		jellyfinCheck.test = client.Ping
	}
	pushCheck := connectionCheck{service: "push"}
	if credentials := s.currentSettings().APNs.Credentials; len(credentials) > 0 {
		pushCheck.test = func(context.Context) (string, error) {
			for _, credential := range credentials {
				if err := push.CheckCredential(credential); err != nil {
					return "", err
				}
			}
			return fmt.Sprintf("%d APNs key(s) ready", len(credentials)), nil
		}
	}
	return append(checks, radarr, sonarr, prowlarr, tmdbCheck, jellyfinCheck, pushCheck)
}
