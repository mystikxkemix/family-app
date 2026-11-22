package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"familyapp/internal/arr"
	"familyapp/internal/auth"
	"familyapp/internal/config"
	"familyapp/internal/push"
	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// These tests need a MongoDB. Set TEST_MONGODB_URI, for example:
//
//	docker run -d --rm -p 27099:27017 mongo:8
//	TEST_MONGODB_URI=mongodb://localhost:27099 go test ./...
//
// Each run uses its own database and drops it at the end.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	return newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin})
}

// testAdmin is the first account, as if set by ADMIN_USERNAME / ADMIN_PASSWORD.
var testAdmin = AdminAccount{Username: "Alex", Password: "initial-password"}

func newTestServerWith(t *testing.T, options Options) (*Server, *httptest.Server) {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("TEST_MONGODB_URI is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, uri, "family_test_"+bson.NewObjectID().Hex())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.DropDatabase(ctx)
		_ = st.Close(ctx)
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, err := New(ctx, st, push.NewNotifier(st, nil, log), log, options)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanups run last-in first-out: the server stops before the database is dropped.
	t.Cleanup(server.Close)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return server, httpServer
}

type client struct {
	t     *testing.T
	base  string
	token string
}

// call sends a request and decodes the JSON answer into out (if not nil).
func (c *client) call(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, c.base+path, reader)
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: want %d, got %d: %s", method, path, wantStatus, response.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, data, err)
		}
	}
}

func TestEndToEnd(t *testing.T) {
	server, httpServer := newTestServer(t)
	// This scenario makes many logins from one IP; TestLoginIsRateLimited
	// covers the limit itself.
	server.loginLimiter = auth.NewLimiter(1000, 1000)
	anonymous := &client{t: t, base: httpServer.URL}
	// --- First login: the admin from the environment must change its password. ---
	var alexAuth authResponse
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "alex", "password": testAdmin.Password}, 200, &alexAuth)
	if !alexAuth.User.MustChangePassword {
		t.Fatalf("the env admin must change its password: %+v", alexAuth.User)
	}
	alex := &client{t: t, base: httpServer.URL, token: alexAuth.Token}
	alex.call("GET", "/api/v1/lists", nil, 403, nil)
	alex.call("POST", "/api/v1/household/invites", nil, 403, nil)
	var pending meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &pending)
	if !pending.User.MustChangePassword || pending.Household.Name != "Home" {
		t.Fatalf("me before the change: %+v", pending)
	}
	samePassword := map[string]string{"currentPassword": testAdmin.Password, "newPassword": testAdmin.Password}
	alex.call("POST", "/api/v1/me/password", samePassword, 400, nil)
	alex.call("POST", "/api/v1/me/password", map[string]string{"currentPassword": testAdmin.Password, "newPassword": "password-1"}, 204, nil)
	alex.call("GET", "/api/v1/lists", nil, 200, nil)
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "alex", "password": testAdmin.Password}, 401, nil)
	alex.call("PATCH", "/api/v1/household", map[string]string{"name": "Our home"}, 200, nil)

	// --- No signup without an invite. ---
	anonymous.call("POST", "/api/v1/auth/register", map[string]string{"username": "eve", "password": "password-9", "displayName": "Eve"}, 400, nil)
	anonymous.call("POST", "/api/v1/auth/register", map[string]string{"username": "eve", "password": "password-9", "displayName": "Eve", "inviteCode": "WRONG-CODE-0000"}, 403, nil)

	// --- Invite the partner. ---
	var invite inviteResponse
	alex.call("POST", "/api/v1/household/invites", nil, 201, &invite)
	partnerSignup := map[string]string{"username": "sam", "password": "password-2", "displayName": "Sam", "inviteCode": invite.Code}
	takenName := map[string]string{"username": "alex", "password": "password-2", "displayName": "X", "inviteCode": invite.Code}
	anonymous.call("POST", "/api/v1/auth/register", takenName, 409, nil) // must not burn the invite
	var samAuth authResponse
	anonymous.call("POST", "/api/v1/auth/register", partnerSignup, 201, &samAuth)
	partnerSignup["username"] = "sam2"
	anonymous.call("POST", "/api/v1/auth/register", partnerSignup, 403, nil) // invite is one-time
	sam := &client{t: t, base: httpServer.URL, token: samAuth.Token}

	var me meResponse
	sam.call("GET", "/api/v1/me", nil, 200, &me)
	if me.Household.Name != "Our home" || len(me.Members) != 2 {
		t.Fatalf("partner must join the household: %+v", me)
	}

	// --- Login. ---
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "alex", "password": "nope-nope"}, 401, nil)
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "ghost", "password": "nope-nope"}, 401, nil)
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": " ALEX ", "password": "password-1"}, 200, nil)
	anonymous.call("GET", "/api/v1/lists", nil, 401, nil)
	(&client{t: t, base: httpServer.URL, token: "forged"}).call("GET", "/api/v1/lists", nil, 401, nil)

	// --- Lists and items, shared between both members. ---
	var list store.List
	alex.call("POST", "/api/v1/lists", map[string]string{"name": "Groceries", "kind": "grocery", "emoji": "🛒"}, 201, &list)
	alex.call("POST", "/api/v1/lists", map[string]string{"name": "Bad", "kind": "nope"}, 400, nil)
	var milk, eggs store.ListItem
	alex.call("POST", "/api/v1/lists/"+list.ID.Hex()+"/items", map[string]string{"text": "Milk"}, 201, &milk)
	alex.call("POST", "/api/v1/lists/"+list.ID.Hex()+"/items", map[string]string{"text": "Eggs", "note": "x12"}, 201, &eggs)
	alex.call("POST", "/api/v1/lists/"+list.ID.Hex()+"/items", map[string]string{"text": "  "}, 400, nil)
	alex.call("POST", "/api/v1/lists/"+list.ID.Hex()+"/items", map[string]any{"text": "x", "unknown": 1}, 400, nil)

	var checked store.ListItem
	sam.call("PATCH", "/api/v1/lists/"+list.ID.Hex()+"/items/"+milk.ID.Hex(), map[string]bool{"checked": true}, 200, &checked)
	if !checked.Checked || checked.CheckedBy == nil || *checked.CheckedBy != samAuth.User.ID {
		t.Fatalf("check must record who checked: %+v", checked)
	}
	var lists []store.List
	sam.call("GET", "/api/v1/lists", nil, 200, &lists)
	if len(lists) != 1 || lists[0].TotalCount != 2 || lists[0].OpenCount != 1 {
		t.Fatalf("bad counts: %+v", lists)
	}
	var cleared map[string]int64
	sam.call("POST", "/api/v1/lists/"+list.ID.Hex()+"/clear-checked", nil, 200, &cleared)
	var items []store.ListItem
	sam.call("GET", "/api/v1/lists/"+list.ID.Hex()+"/items", nil, 200, &items)
	if cleared["deleted"] != 1 || len(items) != 1 || items[0].Text != "Eggs" {
		t.Fatalf("clear-checked: %v %+v", cleared, items)
	}

	// --- Another household must see nothing. ---
	stranger := strangerClient(t, server, httpServer.URL)
	stranger.call("GET", "/api/v1/lists/"+list.ID.Hex()+"/items", nil, 404, nil)
	stranger.call("DELETE", "/api/v1/lists/"+list.ID.Hex(), nil, 404, nil)
	var strangerLists []store.List
	stranger.call("GET", "/api/v1/lists", nil, 200, &strangerLists)
	if len(strangerLists) != 0 {
		t.Fatal("household isolation broken")
	}

	// --- Watchlist. ---
	var movie store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Arrival", "year": 2016, "externalIds": map[string]int{"tmdb": 329865}}, 201, &movie)
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "X", "posterUrl": "http://insecure"}, 400, nil)
	if movie.Status != store.MediaStatusToWatch || len(movie.InterestedUserIDs) != 1 {
		t.Fatalf("new media defaults: %+v", movie)
	}
	sam.call("PATCH", "/api/v1/watchlist/"+movie.ID.Hex(), map[string]any{"interested": true}, 200, &movie)
	if len(movie.InterestedUserIDs) != 2 {
		t.Fatalf("interest not added: %+v", movie)
	}
	sam.call("PATCH", "/api/v1/watchlist/"+movie.ID.Hex(), map[string]any{"status": "watched"}, 200, &movie)
	if movie.WatchedAt == nil {
		t.Fatal("watchedAt not set")
	}
	var toWatch []store.Media
	sam.call("GET", "/api/v1/watchlist?status=toWatch", nil, 200, &toWatch)
	if len(toWatch) != 0 {
		t.Fatal("status filter broken")
	}

	// --- Events. ---
	start := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	var event store.Event
	alex.call("POST", "/api/v1/events", map[string]any{"title": "Dinner", "startsAt": start, "location": "Home"}, 201, &event)
	alex.call("POST", "/api/v1/events", map[string]any{"title": "Bad", "startsAt": start, "endsAt": start.Add(-time.Hour)}, 400, nil)
	alex.call("POST", "/api/v1/events", map[string]any{"title": "Old", "startsAt": start.Add(-30 * 24 * time.Hour)}, 201, nil)
	var events []store.Event
	sam.call("GET", "/api/v1/events", nil, 200, &events)
	if len(events) != 1 || events[0].Title != "Dinner" {
		t.Fatalf("events from now: %+v", events)
	}
	sam.call("PATCH", "/api/v1/events/"+event.ID.Hex(), map[string]any{"endsAt": start.Add(2 * time.Hour)}, 200, &event)
	var cleared2 store.Event // fresh value: an omitted field would keep the old one
	sam.call("PATCH", "/api/v1/events/"+event.ID.Hex(), map[string]any{"clearEnd": true}, 200, &cleared2)
	if event.EndsAt == nil || cleared2.EndsAt != nil {
		t.Fatal("endsAt not set, then not removed by clearEnd")
	}

	// --- Devices. ---
	token := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	var device registerDeviceResponse
	sam.call("PUT", "/api/v1/devices/"+token, map[string]string{"topic": "app.one", "environment": "sandbox"}, 200, &device)
	sam.call("PUT", "/api/v1/devices/not-hex", map[string]string{"topic": "app.one", "environment": "sandbox"}, 400, nil)
	sam.call("PUT", "/api/v1/devices/"+token, map[string]string{"topic": "app.one", "environment": "beta"}, 400, nil)
	sam.call("DELETE", "/api/v1/devices/"+token, nil, 204, nil)

	// --- Password change logs out the other sessions; logout ends this one. ---
	var second authResponse
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "sam", "password": "password-2"}, 200, &second)
	sam.call("POST", "/api/v1/me/password", map[string]string{"currentPassword": "wrong-one", "newPassword": "password-3"}, 403, nil)
	sam.call("POST", "/api/v1/me/password", map[string]string{"currentPassword": "password-2", "newPassword": "password-3"}, 204, nil)
	(&client{t: t, base: httpServer.URL, token: second.Token}).call("GET", "/api/v1/me", nil, 401, nil)
	sam.call("GET", "/api/v1/me", nil, 200, nil)
	sam.call("POST", "/api/v1/auth/logout", nil, 204, nil)
	sam.call("GET", "/api/v1/me", nil, 401, nil)
}

// strangerClient creates a user in a separate household, straight in the store.
func strangerClient(t *testing.T, server *Server, base string) *client {
	t.Helper()
	ctx := context.Background()
	household := &store.Household{Name: "Other"}
	if err := server.store.CreateHousehold(ctx, household); err != nil {
		t.Fatal(err)
	}
	user := &store.User{Username: "stranger", DisplayName: "Stranger", HouseholdID: household.ID, PasswordHash: "x"}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	if err := server.startSession(recorder, httptest.NewRequest("POST", "/api/v1/auth/login", nil), user, 200); err != nil {
		t.Fatal(err)
	}
	var answer authResponse
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	return &client{t: t, base: base, token: answer.Token}
}

func TestLoginIsRateLimited(t *testing.T) {
	_, httpServer := newTestServer(t)
	anonymous := &client{t: t, base: httpServer.URL}
	wrong := map[string]string{"username": "nobody", "password": "wrong-password"}
	for range 10 {
		anonymous.call("POST", "/api/v1/auth/login", wrong, 401, nil)
	}
	anonymous.call("POST", "/api/v1/auth/login", wrong, 429, nil)
}

// signUp logs in as the env admin, changes its password and returns its client.
func signUp(t *testing.T, server *Server, base string) *client {
	t.Helper()
	anonymous := &client{t: t, base: base}
	var answer authResponse
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": testAdmin.Username, "password": testAdmin.Password}, 200, &answer)
	alex := &client{t: t, base: base, token: answer.Token}
	alex.call("POST", "/api/v1/me/password", map[string]string{"currentPassword": testAdmin.Password, "newPassword": "password-1"}, 204, nil)
	return alex
}

func TestAdminIsRequiredOnEmptyDatabase(t *testing.T) {
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("TEST_MONGODB_URI is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, uri, "family_test_"+bson.NewObjectID().Hex())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.DropDatabase(ctx); _ = st.Close(ctx) }()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, admin := range []AdminAccount{{Username: "admin"}, {Username: "admin", Password: "short"}, {Username: "x", Password: "long-enough"}} {
		if _, err := New(ctx, st, push.NewNotifier(st, nil, log), log, Options{Admin: admin}); err == nil {
			t.Fatalf("admin %+v must be refused", admin)
		}
	}
}

func TestPlaces(t *testing.T) {
	server, httpServer := newTestServer(t)
	alex := signUp(t, server, httpServer.URL)

	var place store.Place
	alex.call("POST", "/api/v1/places", map[string]any{"name": "Chez Lulu", "category": "restaurant", "address": "1 rue X", "url": "https://lulu.example"}, 201, &place)
	if place.Status != store.PlaceStatusToTry || len(place.InterestedUserIDs) != 1 {
		t.Fatalf("new place defaults: %+v", place)
	}
	alex.call("POST", "/api/v1/places", map[string]any{"name": "X", "category": "spa"}, 400, nil)
	alex.call("POST", "/api/v1/places", map[string]any{"name": "X", "url": "javascript:alert(1)"}, 400, nil)

	var tried store.Place
	alex.call("PATCH", "/api/v1/places/"+place.ID.Hex(), map[string]any{"status": "tried", "rating": 4, "interested": false}, 200, &tried)
	if tried.TriedAt == nil || tried.Rating != 4 || len(tried.InterestedUserIDs) != 0 {
		t.Fatalf("tried update: %+v", tried)
	}
	alex.call("PATCH", "/api/v1/places/"+place.ID.Hex(), map[string]any{"rating": 6}, 400, nil)

	var toTry []store.Place
	alex.call("GET", "/api/v1/places?status=toTry", nil, 200, &toTry)
	var all []store.Place
	alex.call("GET", "/api/v1/places", nil, 200, &all)
	if len(toTry) != 0 || len(all) != 1 {
		t.Fatalf("filter: toTry=%d all=%d", len(toTry), len(all))
	}
	strangerClient(t, server, httpServer.URL).call("DELETE", "/api/v1/places/"+place.ID.Hex(), nil, 404, nil)
	alex.call("DELETE", "/api/v1/places/"+place.ID.Hex(), nil, 204, nil)
}

func TestMediaToolsNotConfigured(t *testing.T) {
	server, httpServer := newTestServer(t)
	alex := signUp(t, server, httpServer.URL)
	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if me.Integrations.Radarr || me.Integrations.Prowlarr {
		t.Fatalf("no tool is configured: %+v", me.Integrations)
	}
	alex.call("GET", "/api/v1/search/media?kind=movie&q=arrival", nil, 503, nil)
	alex.call("GET", "/api/v1/releases?q=arrival", nil, 503, nil)
}

func TestMediaTools(t *testing.T) {
	var posted map[string]any
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v3/movie/lookup":
			_, _ = w.Write([]byte(`[{"title":"Arrival","year":2016,"tmdbId":1,"remotePoster":"https://img/p.jpg"}]`))
		case "GET /api/v3/qualityprofile":
			_, _ = w.Write([]byte(`[{"id":1}]`))
		case "GET /api/v3/rootfolder":
			_, _ = w.Write([]byte(`[{"path":"/movies"}]`))
		case "POST /api/v3/movie":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			w.WriteHeader(http.StatusCreated)
		case "GET /api/v1/search":
			_, _ = w.Write([]byte(`[{"guid":"g1","indexerId":2,"title":"Arrival.2016","size":1,"protocol":"torrent","publishDate":"2020-01-02T03:04:05Z"}]`))
		case "POST /api/v1/search":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	cfg := config.ArrConfig{URL: fake.URL, APIKey: "k"}
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Settings: config.Settings{Radarr: cfg, Prowlarr: cfg}})
	alex := signUp(t, server, httpServer.URL)

	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.Integrations.Radarr || me.Integrations.Sonarr || !me.Integrations.Prowlarr {
		t.Fatalf("integrations: %+v", me.Integrations)
	}

	var results []arr.SearchResult
	alex.call("GET", "/api/v1/search/media?kind=movie&q=arrival", nil, 200, &results)
	if len(results) != 1 || results[0].ExternalIDs.TMDB != 1 {
		t.Fatalf("search: %+v", results)
	}
	alex.call("GET", "/api/v1/search/media?kind=show&q=x", nil, 503, nil)

	var movie store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Arrival", "posterUrl": results[0].PosterURL, "externalIds": results[0].ExternalIDs}, 201, &movie)
	var download struct {
		Media store.Media `json:"media"`
	}
	alex.call("POST", "/api/v1/watchlist/"+movie.ID.Hex()+"/download", nil, 200, &download)
	if download.Media.DownloadRequestedAt == nil || posted["tmdbId"] != float64(1) {
		t.Fatalf("download: %+v posted=%v", download.Media, posted)
	}
	var noIDs store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Home video"}, 201, &noIDs)
	alex.call("POST", "/api/v1/watchlist/"+noIDs.ID.Hex()+"/download", nil, 400, nil)

	var releases []arr.Release
	alex.call("GET", "/api/v1/releases?kind=movie&q=arrival", nil, 200, &releases)
	if len(releases) != 1 {
		t.Fatalf("releases: %+v", releases)
	}
	alex.call("POST", "/api/v1/releases/grab", map[string]any{"guid": "g1", "indexerId": 2}, 204, nil)
	alex.call("POST", "/api/v1/releases/grab", map[string]any{"guid": "", "indexerId": 2}, 400, nil)
}

// webClient talks like the website: cookie session and the X-Requested-With header.
type webClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	header bool
}

func newWebClient(t *testing.T, base string) *webClient {
	jar, _ := cookiejar.New(nil)
	return &webClient{t: t, base: base, http: &http.Client{Jar: jar}, header: true}
}

func (c *webClient) call(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, c.base+path, reader)
	if c.header {
		request.Header.Set("X-Requested-With", "family-web")
	}
	response, err := c.http.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: want %d, got %d: %s", method, path, wantStatus, response.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("decode %s: %v", data, err)
		}
	}
}

func TestWebsiteCookieLogin(t *testing.T) {
	server, httpServer := newTestServer(t)
	signUp(t, server, httpServer.URL) // admin password becomes password-1

	web := newWebClient(t, httpServer.URL)
	var login authResponse
	web.call("POST", "/api/v1/auth/login", map[string]string{"username": "alex", "password": "password-1"}, 200, &login)
	if login.Token != "" {
		t.Fatal("the website must never get the token in JSON")
	}
	var me meResponse
	web.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.User.IsAdmin {
		t.Fatal("the env admin must be an admin")
	}

	// The cookie alone, without the header, is refused (CSRF).
	web.header = false
	web.call("GET", "/api/v1/me", nil, 403, nil)
	web.header = true

	web.call("POST", "/api/v1/auth/logout", nil, 204, nil)
	web.call("GET", "/api/v1/me", nil, 401, nil)
}

func TestAdminUsers(t *testing.T) {
	server, httpServer := newTestServer(t)
	alex := signUp(t, server, httpServer.URL)

	var invite inviteResponse
	alex.call("POST", "/api/v1/household/invites", nil, 201, &invite)
	anonymous := &client{t: t, base: httpServer.URL}
	var samAuth authResponse
	anonymous.call("POST", "/api/v1/auth/register", map[string]string{"username": "sam", "password": "password-2", "displayName": "Sam", "inviteCode": invite.Code}, 201, &samAuth)
	sam := &client{t: t, base: httpServer.URL, token: samAuth.Token}

	// A member is not an admin.
	sam.call("GET", "/api/v1/admin/users", nil, 403, nil)

	var users []adminUser
	alex.call("GET", "/api/v1/admin/users", nil, 200, &users)
	if len(users) != 2 || users[0].HouseholdName != "Home" || !users[0].IsAdmin || users[1].IsAdmin {
		t.Fatalf("admin users: %+v", users)
	}
	alexID, samID := users[0].ID.Hex(), users[1].ID.Hex()

	// The last admin cannot be removed, but can promote someone first.
	alex.call("PATCH", "/api/v1/admin/users/"+alexID, map[string]bool{"isAdmin": false}, 409, nil)
	alex.call("PATCH", "/api/v1/admin/users/"+samID, map[string]bool{"isAdmin": true}, 200, nil)
	sam.call("GET", "/api/v1/admin/status", nil, 200, nil)
	sam.call("PATCH", "/api/v1/admin/users/"+samID, map[string]bool{"isAdmin": false}, 200, nil)

	// Reset password: old sessions end, the temporary password must be changed.
	var reset resetPasswordResponse
	alex.call("POST", "/api/v1/admin/users/"+samID+"/reset-password", nil, 200, &reset)
	sam.call("GET", "/api/v1/me", nil, 401, nil)
	var again authResponse
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "sam", "password": reset.TemporaryPassword}, 200, &again)
	if !again.User.MustChangePassword {
		t.Fatal("a reset password must be changed at the next login")
	}

	alex.call("DELETE", "/api/v1/admin/users/"+alexID, nil, 400, nil)
	alex.call("DELETE", "/api/v1/admin/users/"+samID, nil, 204, nil)
	anonymous.call("POST", "/api/v1/auth/login", map[string]string{"username": "sam", "password": reset.TemporaryPassword}, 401, nil)
}

func TestAdminSettings(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "radarr-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"appName":"Radarr","version":"5.1"}`))
	}))
	defer fake.Close()

	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, SettingsFile: settingsFile})
	alex := signUp(t, server, httpServer.URL)

	var status adminStatus
	alex.call("GET", "/api/v1/admin/status", nil, 200, &status)
	if status.SettingsFound || !status.SettingsWritable || status.Users != 1 {
		t.Fatalf("status: %+v", status)
	}

	// Save Radarr: applied at once, written to the file, key never sent back.
	body := map[string]any{
		"radarr":   map[string]any{"url": fake.URL + "/", "apiKey": "radarr-key", "qualityProfileId": 2},
		"sonarr":   map[string]any{"url": ""},
		"prowlarr": map[string]any{"url": ""},
		"apns":     map[string]any{"credentials": []any{}},
	}
	var saved adminSettings
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if !saved.Radarr.APIKeySet || saved.Radarr.APIKey != "" || saved.Radarr.URL != fake.URL {
		t.Fatalf("saved settings: %+v", saved)
	}
	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.Integrations.Radarr {
		t.Fatal("new settings must apply without a restart")
	}
	onDisk, found, err := config.LoadSettings(settingsFile)
	if err != nil || !found || onDisk.Radarr.APIKey != "radarr-key" {
		t.Fatalf("file: %+v found=%v err=%v", onDisk, found, err)
	}
	if info, _ := os.Stat(settingsFile); info.Mode().Perm() != 0o600 {
		t.Fatalf("settings file mode %v, want 0600", info.Mode().Perm())
	}

	// An empty key keeps the saved one; the test call uses it.
	body["radarr"] = map[string]any{"url": fake.URL, "apiKey": ""}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	var test testToolResponse
	alex.call("POST", "/api/v1/admin/settings/test", map[string]string{"tool": "radarr"}, 200, &test)
	if !test.OK || test.Message != "Radarr 5.1 answered" {
		t.Fatalf("test: %+v", test)
	}
	alex.call("POST", "/api/v1/admin/settings/test", map[string]string{"tool": "sonarr"}, 200, &test)
	if test.OK {
		t.Fatal("sonarr is not configured")
	}

	// Validation.
	body["radarr"] = map[string]any{"url": "ftp://x", "apiKey": "k"}
	alex.call("PUT", "/api/v1/admin/settings", body, 400, nil)
	body["radarr"] = map[string]any{"url": ""}
	body["apns"] = map[string]any{"credentials": []any{map[string]any{"teamId": "ABCDE12345", "keyId": "KEY0000001", "topics": []string{"a.b"}, "key": "not a key"}}}
	alex.call("PUT", "/api/v1/admin/settings", body, 400, nil)

	// A valid .p8 key is written next to the settings file and turns push on.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	body["apns"] = map[string]any{"credentials": []any{map[string]any{"teamId": "ABCDE12345", "keyId": "KEY0000001", "topics": []string{"a.b"}, "key": pemKey}}}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if len(saved.APNs.Credentials) != 1 || !saved.APNs.Credentials[0].KeySet || saved.APNs.Credentials[0].Key != "" {
		t.Fatalf("apns: %+v", saved.APNs)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(settingsFile), "apns", "AuthKey_KEY0000001.p8")); err != nil {
		t.Fatal("key file not written")
	}
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.PushEnabled || me.Integrations.Radarr {
		t.Fatalf("push on, radarr off expected: %+v", me)
	}
}

func TestAdminSettingsNotWritable(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(directory, 0o700)
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, SettingsFile: filepath.Join(directory, "settings.json")})
	alex := signUp(t, server, httpServer.URL)
	body := map[string]any{"radarr": map[string]any{"url": ""}, "sonarr": map[string]any{"url": ""}, "prowlarr": map[string]any{"url": ""}, "apns": map[string]any{"credentials": []any{}}}
	alex.call("PUT", "/api/v1/admin/settings", body, 409, nil)
}

func TestEnsureAdminPromotesOldAccounts(t *testing.T) {
	server, httpServer := newTestServer(t)
	signUp(t, server, httpServer.URL)
	ctx := context.Background()
	admin, _ := server.store.UserByUsername(ctx, "alex")
	if _, err := server.store.UpdateUser(ctx, admin.ID, bson.M{"isAdmin": false}); err != nil {
		t.Fatal(err)
	}
	// A database from before the admin role: the next start promotes the env admin.
	if err := server.ensureAdmin(ctx, AdminAccount{Username: "Alex"}); err != nil {
		t.Fatal(err)
	}
	admin, _ = server.store.UserByUsername(ctx, "alex")
	if !admin.IsAdmin {
		t.Fatal("the env admin account was not promoted")
	}
}

func TestWebsiteRoutes(t *testing.T) {
	_, httpServer := newTestServer(t)
	response, err := http.Get(httpServer.URL + "/lists/some-id")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	// Without a build the website answers 404 with a hint; with one, index.html.
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		t.Fatalf("website route: %d", response.StatusCode)
	}
	if csp := response.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("website CSP: %s", csp)
	}
	response, err = http.Get(httpServer.URL + "/api/v1/nope")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("unknown API path: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if csp := response.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("API CSP: %s", csp)
	}
}

// fakeTMDB answers search, details and configuration for one movie.
func fakeTMDB(t *testing.T, detailCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "tmdb-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status_message":"Invalid API key"}`))
			return
		}
		switch r.URL.Path {
		case "/configuration":
			_, _ = w.Write([]byte(`{"images":{}}`))
		case "/search/movie":
			if strings.EqualFold(r.URL.Query().Get("query"), "arrival") || strings.EqualFold(r.URL.Query().Get("query"), "premier contact") {
				// Like TMDB: the title is in the requested language.
				title := "Arrival"
				if r.URL.Query().Get("language") == "fr-FR" {
					title = "Premier Contact"
				}
				_, _ = w.Write([]byte(`{"results":[{"id":329865,"title":"` + title + `","original_title":"Arrival","release_date":"2016-11-10","poster_path":"/p.jpg"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/movie/329865":
			if detailCalls != nil {
				detailCalls.Add(1)
			}
			title := "Arrival"
			if r.URL.Query().Get("language") == "fr-FR" {
				title = "Premier Contact"
			}
			_, _ = w.Write([]byte(`{"id":329865,"title":"` + title + `","original_title":"Arrival","release_date":"2016-11-10","runtime":116,"vote_average":7.58,"vote_count":18000,"poster_path":"/p.jpg","external_ids":{"imdb_id":"tt2543164"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status_message":"not found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestMediaDetails(t *testing.T) {
	var calls atomic.Int32
	fake := fakeTMDB(t, &calls)
	matchDelay = time.Millisecond
	settings := config.Settings{TMDB: config.TMDBConfig{APIKey: "tmdb-key"}}
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Settings: settings, TMDBBaseURL: fake.URL})
	alex := signUp(t, server, httpServer.URL)

	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if !me.Integrations.TMDB {
		t.Fatal("integrations.tmdb must be true")
	}

	// Search goes to TMDB when it is set up.
	var results []arr.SearchResult
	alex.call("GET", "/api/v1/search/media?kind=movie&q=arrival&lang=en", nil, 200, &results)
	if len(results) != 1 || results[0].ExternalIDs.TMDB != 329865 {
		t.Fatalf("search: %+v", results)
	}

	// A title picked from the search: details in each language, cached.
	var movie store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Arrival", "externalIds": map[string]int{"tmdb": 329865}}, 201, &movie)
	var details struct {
		Title      string  `json:"title"`
		TMDBRating float64 `json:"tmdbRating"`
		IMDBID     string  `json:"imdbId"`
	}
	alex.call("GET", "/api/v1/watchlist/"+movie.ID.Hex()+"/details?lang=fr", nil, 200, &details)
	if details.Title != "Premier Contact" || details.TMDBRating != 7.6 {
		t.Fatalf("French details: %+v", details)
	}
	alex.call("GET", "/api/v1/watchlist/"+movie.ID.Hex()+"/details?lang=fr", nil, 200, &details)
	alex.call("GET", "/api/v1/watchlist/"+movie.ID.Hex()+"/details?lang=en", nil, 200, &details)
	if details.Title != "Arrival" || calls.Load() != 2 {
		t.Fatalf("cache: title=%s TMDB detail calls=%d (want 2: fr once, en once)", details.Title, calls.Load())
	}
	// The details filled the IMDb id of the title.
	var list []store.Media
	alex.call("GET", "/api/v1/watchlist", nil, 200, &list)
	if len(list) != 1 || list[0].ExternalIDs.IMDB != "tt2543164" {
		t.Fatalf("ids not saved: %+v", list)
	}

	// A title typed by hand is linked automatically (safe match).
	var typed store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Premier contact", "year": 2016}, 201, &typed)
	var unknown store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Our holiday video"}, 201, &unknown)
	deadline := time.Now().Add(5 * time.Second)
	for {
		// A fresh slice each time: decoding into the old one keeps omitted fields.
		var current []store.Media
		alex.call("GET", "/api/v1/watchlist", nil, 200, &current)
		list = current
		linked := 0
		for _, media := range list {
			if media.ID == typed.ID && media.ExternalIDs.TMDB == 329865 && media.PosterURL != "" {
				linked++
			}
		}
		if linked == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("automatic match did not happen: %+v", list)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// No safe match: not linked, details answer not_matched.
	var failure map[string]string
	alex.call("GET", "/api/v1/watchlist/"+unknown.ID.Hex()+"/details", nil, 404, &failure)
	if failure["code"] != "not_matched" {
		t.Fatalf("want not_matched, got %v", failure)
	}

	// Manual match ("Find the details").
	var matched store.Media
	alex.call("POST", "/api/v1/watchlist/"+unknown.ID.Hex()+"/match", map[string]int{"tmdbId": 329865}, 200, &matched)
	if matched.ExternalIDs.TMDB != 329865 || matched.Title != "Our holiday video" || matched.Year != 2016 {
		t.Fatalf("match must keep the title and fill ids/year: %+v", matched)
	}
	alex.call("POST", "/api/v1/watchlist/"+unknown.ID.Hex()+"/match", map[string]int{"tmdbId": 0}, 400, nil)
	strangerClient(t, server, httpServer.URL).call("GET", "/api/v1/watchlist/"+movie.ID.Hex()+"/details", nil, 404, nil)
}

func TestMediaDetailsNotConfigured(t *testing.T) {
	server, httpServer := newTestServer(t)
	alex := signUp(t, server, httpServer.URL)
	var movie store.Media
	alex.call("POST", "/api/v1/watchlist", map[string]any{"kind": "movie", "title": "Arrival", "externalIds": map[string]int{"tmdb": 1}}, 201, &movie)
	alex.call("GET", "/api/v1/watchlist/"+movie.ID.Hex()+"/details", nil, 503, nil)
	alex.call("POST", "/api/v1/watchlist/"+movie.ID.Hex()+"/match", map[string]int{"tmdbId": 1}, 503, nil)
}

func TestAdminTMDBKey(t *testing.T) {
	fake := fakeTMDB(t, nil)
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, SettingsFile: settingsFile, TMDBBaseURL: fake.URL})
	alex := signUp(t, server, httpServer.URL)
	body := map[string]any{"radarr": map[string]any{"url": ""}, "sonarr": map[string]any{"url": ""}, "prowlarr": map[string]any{"url": ""},
		"tmdb": map[string]any{"apiKey": "tmdb-key"}, "apns": map[string]any{"credentials": []any{}}}
	var saved adminSettings
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if !saved.TMDB.APIKeySet || saved.TMDB.APIKey != "" {
		t.Fatalf("tmdb settings: %+v", saved.TMDB)
	}
	var test testToolResponse
	alex.call("POST", "/api/v1/admin/settings/test", map[string]string{"tool": "tmdb"}, 200, &test)
	if !test.OK {
		t.Fatalf("tmdb test: %+v", test)
	}
	// "" keeps the key; remove deletes it.
	body["tmdb"] = map[string]any{"apiKey": ""}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	if !saved.TMDB.APIKeySet {
		t.Fatal("an empty key must keep the saved one")
	}
	body["tmdb"] = map[string]any{"remove": true}
	alex.call("PUT", "/api/v1/admin/settings", body, 200, &saved)
	var me meResponse
	alex.call("GET", "/api/v1/me", nil, 200, &me)
	if saved.TMDB.APIKeySet || me.Integrations.TMDB {
		t.Fatal("remove must delete the key and turn TMDB off")
	}
}

func TestAdminConnections(t *testing.T) {
	arrFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"appName":"Radarr","version":"5.14"}`))
	}))
	defer arrFake.Close()
	tmdbFake := fakeTMDB(t, nil)

	// A valid APNs key on disk.
	directory := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	keyFile := filepath.Join(directory, "AuthKey_KEY0000001.p8")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}

	settings := config.Settings{
		Radarr:   config.ArrConfig{URL: arrFake.URL, APIKey: "good"},
		Sonarr:   config.ArrConfig{URL: arrFake.URL, APIKey: "wrong"},
		Prowlarr: config.ArrConfig{URL: "http://127.0.0.1:1", APIKey: "x"},
		TMDB:     config.TMDBConfig{APIKey: "tmdb-key"},
		APNs:     push.Config{Credentials: []push.Credential{{TeamID: "ABCDE12345", KeyID: "KEY0000001", KeyFile: keyFile, Topics: []string{"a.b"}}}},
	}
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Settings: settings, TMDBBaseURL: tmdbFake.URL})
	alex := signUp(t, server, httpServer.URL)

	var results []connectionResult
	alex.call("GET", "/api/v1/admin/connections", nil, 200, &results)
	byService := map[string]connectionResult{}
	order := []string{}
	for _, result := range results {
		byService[result.Service] = result
		order = append(order, result.Service)
	}
	if strings.Join(order, ",") != "database,radarr,sonarr,prowlarr,tmdb,push" {
		t.Fatalf("order: %v", order)
	}
	expect := func(service string, configured, ok bool, contains string) {
		t.Helper()
		got := byService[service]
		if got.Configured != configured || got.OK != ok || !strings.Contains(got.Message, contains) {
			t.Fatalf("%s: %+v", service, got)
		}
	}
	expect("database", true, true, "MongoDB")
	expect("radarr", true, true, "Radarr 5.14")
	expect("sonarr", true, false, "401")
	expect("prowlarr", true, false, "not reachable")
	expect("tmdb", true, true, "TMDB")
	expect("push", true, true, "1 APNs key")
	if strings.Contains(byService["sonarr"].Message, "wrong") {
		t.Fatal("a message must never contain the key")
	}

	// A member cannot run the tests.
	var invite inviteResponse
	alex.call("POST", "/api/v1/household/invites", nil, 201, &invite)
	var samAuth authResponse
	(&client{t: t, base: httpServer.URL}).call("POST", "/api/v1/auth/register", map[string]string{"username": "sam", "password": "password-2", "displayName": "Sam", "inviteCode": invite.Code}, 201, &samAuth)
	(&client{t: t, base: httpServer.URL, token: samAuth.Token}).call("GET", "/api/v1/admin/connections", nil, 403, nil)

	// Nothing set up: every tool says so, the database still answers.
	bare, bareServer := newTestServer(t)
	admin := signUp(t, bare, bareServer.URL)
	admin.call("GET", "/api/v1/admin/connections", nil, 200, &results)
	for _, result := range results {
		if result.Service != "database" && (result.Configured || result.Message != "not set up") {
			t.Fatalf("not set up expected: %+v", result)
		}
	}
}
