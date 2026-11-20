package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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
	if err := server.startSession(recorder, ctx, user, 200); err != nil {
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
	server, httpServer := newTestServerWith(t, Options{SessionTTL: time.Hour, Admin: testAdmin, Arr: arr.New(cfg, config.ArrConfig{}, cfg)})
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
