package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSignES256(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	token, err := signES256(key, "KEY123", "TEAM456", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a compact JWT: %s", token)
	}
	encoding := base64.RawURLEncoding
	header, _ := encoding.DecodeString(parts[0])
	claims, _ := encoding.DecodeString(parts[1])
	if string(header) != `{"alg":"ES256","kid":"KEY123"}` {
		t.Fatalf("header: %s", header)
	}
	if string(claims) != `{"iat":1700000000,"iss":"TEAM456"}` {
		t.Fatalf("claims: %s", claims)
	}
	signature, _ := encoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
		t.Fatal("signature does not verify")
	}
}

func TestSignerReusesToken(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s := &signer{teamID: "T", keyID: "K", key: key}
	start := time.Now()
	first, _ := s.bearer(start)
	second, _ := s.bearer(start.Add(10 * time.Minute))
	third, _ := s.bearer(start.Add(time.Hour))
	if first != second {
		t.Fatal("token must be reused within its lifetime")
	}
	if first == third {
		t.Fatal("token must be renewed after its lifetime")
	}
}

func TestSendMapsResponses(t *testing.T) {
	var gotPath, gotTopic, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTopic, gotAuth = r.URL.Path, r.Header.Get("apns-topic"), r.Header.Get("authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		if strings.HasSuffix(r.URL.Path, "/dead") {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
		}
	}))
	defer server.Close()

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := &Client{
		http:    server.Client(),
		hosts:   map[string]string{"sandbox": server.URL},
		signers: map[string]*signer{"app.one": {teamID: "T", keyID: "K", key: key}},
	}
	notification := Notification{Title: "Hi", Body: "There", BodyLocKey: "push.k", BodyLocArgs: []string{"Sam"}, ThreadID: "lists", Data: map[string]string{"listId": "abc"}}
	if err := client.Send(context.Background(), "aabb", "app.one", "sandbox", notification); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/3/device/aabb" || gotTopic != "app.one" || !strings.HasPrefix(gotAuth, "bearer ") {
		t.Fatalf("bad request: path=%s topic=%s auth=%s", gotPath, gotTopic, gotAuth)
	}
	alert, _ := gotBody["aps"].(map[string]any)["alert"].(map[string]any)
	if gotBody["listId"] != "abc" || alert["loc-key"] != "push.k" || alert["body"] != "There" {
		t.Fatalf("bad payload: %v", gotBody)
	}
	if _, ok := alert["title-loc-key"]; ok {
		t.Fatal("title-loc-key must be omitted when not set")
	}
	err := client.Send(context.Background(), "dead", "app.one", "sandbox", notification)
	if !errors.Is(err, ErrUnregistered) {
		t.Fatalf("want ErrUnregistered, got %v", err)
	}
	if err := client.Send(context.Background(), "aabb", "other.app", "sandbox", notification); err == nil {
		t.Fatal("unknown topic must fail")
	}
}

type fakeDevices struct {
	mu      sync.Mutex
	devices []store.Device
	deleted []string
}

func (f *fakeDevices) DevicesForHousehold(_ context.Context, _, except bson.ObjectID) ([]store.Device, error) {
	var result []store.Device
	for _, device := range f.devices {
		if device.UserID != except {
			result = append(result, device)
		}
	}
	return result, nil
}

func (f *fakeDevices) DeleteDeviceByToken(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, token)
	return nil
}

type fakeSender struct {
	mu   sync.Mutex
	sent []Notification
}

func (f *fakeSender) Supports(topic string) bool { return topic == "app.one" }

func (f *fakeSender) Send(_ context.Context, token, _, _ string, notification Notification) error {
	if token == "dead" {
		return ErrUnregistered
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, notification)
	return nil
}

func TestNotifierSkipsActorAndBatches(t *testing.T) {
	actor, partner := bson.NewObjectID(), bson.NewObjectID()
	devices := &fakeDevices{devices: []store.Device{
		{Token: "actor", UserID: actor, Topic: "app.one"},
		{Token: "partner", UserID: partner, Topic: "app.one"},
		{Token: "dead", UserID: partner, Topic: "app.one"},
		{Token: "other-app", UserID: partner, Topic: "unknown.app"},
	}}
	sender := &fakeSender{}
	notifier := NewNotifier(devices, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	notifier.batchDelay = time.Hour // Close must flush without waiting

	household := bson.NewObjectID()
	for range 3 {
		notifier.NotifyBatched("k", household, actor, func(count int) Notification {
			return Notification{Body: strings.Repeat("x", count)}
		})
	}
	notifier.Notify(household, actor, Notification{Body: "single"})
	notifier.Close()

	if len(sender.sent) != 2 {
		t.Fatalf("want 2 pushes to the partner (batch + single), got %d: %+v", len(sender.sent), sender.sent)
	}
	bodies := sender.sent[0].Body + "|" + sender.sent[1].Body
	if !strings.Contains(bodies, "xxx") || !strings.Contains(bodies, "single") {
		t.Fatalf("unexpected bodies: %s", bodies)
	}
	if len(devices.deleted) != 2 || devices.deleted[0] != "dead" {
		t.Fatalf("dead token not forgotten: %v", devices.deleted)
	}
}

func TestDisabledNotifierDoesNothing(t *testing.T) {
	notifier := NewNotifier(&fakeDevices{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	notifier.Notify(bson.NewObjectID(), bson.NewObjectID(), Notification{})
	notifier.NotifyBatched("k", bson.NewObjectID(), bson.NewObjectID(), func(int) Notification { return Notification{} })
	notifier.Close()
	if notifier.Enabled() || notifier.Supports("app.one") {
		t.Fatal("notifier without sender must be disabled")
	}
}
