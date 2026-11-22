// Package push sends notifications to iPhones through Apple Push Notification
// service (APNs), with token-based (.p8 key) authentication.
//
// The app is signed by more than one Apple team, and each team has its own
// bundle identifier and its own APNs key. The config therefore maps each
// bundle identifier (the APNs "topic") to the key of its team.
package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// Credential is one APNs key and the bundle identifiers it signs for.
type Credential struct {
	TeamID string `json:"teamId"`
	KeyID  string `json:"keyId"`
	// KeyFile is the path of the AuthKey_<KeyID>.p8 file.
	KeyFile string `json:"keyFile"`
	// Topics are the app bundle identifiers of this team.
	Topics []string `json:"topics"`
}

// Config lists the APNs keys (the "apns" part of the settings file).
type Config struct {
	Credentials []Credential `json:"credentials"`
}

// APNs hosts per environment.
var defaultHosts = map[string]string{
	"sandbox":    "https://api.sandbox.push.apple.com",
	"production": "https://api.push.apple.com",
}

// ErrUnregistered means the device token is no longer valid and should be
// forgotten (app deleted, token for another app...).
var ErrUnregistered = errors.New("device token is no longer valid")

// Notification is what the user sees, plus data the app reads on tap.
//
// Title and Body are the English text. When a LocKey is set, the iPhone shows
// the app's own translation of that key instead (French or English, from the
// app's Localizable strings), filled with the LocArgs.
type Notification struct {
	Title string
	Body  string
	// TitleLocKey and TitleLocArgs localize the title (optional).
	TitleLocKey  string
	TitleLocArgs []string
	// BodyLocKey and BodyLocArgs localize the body (optional).
	BodyLocKey  string
	BodyLocArgs []string
	// ThreadID groups notifications of the same kind in Notification Center.
	ThreadID string
	Data     map[string]string
}

// Client sends notifications to APNs.
type Client struct {
	http    *http.Client
	hosts   map[string]string
	signers map[string]*signer // by topic
}

// NewClient loads every key of the config. Go's HTTP client uses HTTP/2 over
// TLS on its own, which APNs requires.
func NewClient(config Config) (*Client, error) {
	client := &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		hosts:   defaultHosts,
		signers: make(map[string]*signer),
	}
	for _, credential := range config.Credentials {
		pemData, err := os.ReadFile(credential.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read APNs key %s: %w", credential.KeyID, err)
		}
		key, err := parseKey(pemData)
		if err != nil {
			return nil, fmt.Errorf("APNs key %s: %w", credential.KeyID, err)
		}
		if credential.TeamID == "" || credential.KeyID == "" || len(credential.Topics) == 0 {
			return nil, fmt.Errorf("APNs key %s: teamId, keyId and topics are required", credential.KeyID)
		}
		s := &signer{teamID: credential.TeamID, keyID: credential.KeyID, key: key}
		for _, topic := range credential.Topics {
			client.signers[topic] = s
		}
	}
	return client, nil
}

// Supports reports whether a key is configured for this bundle identifier.
func (c *Client) Supports(topic string) bool {
	_, ok := c.signers[topic]
	return ok
}

// Send delivers one notification to one device.
func (c *Client) Send(ctx context.Context, deviceToken, topic, environment string, notification Notification) error {
	s, ok := c.signers[topic]
	if !ok {
		return fmt.Errorf("no APNs key for topic %q", topic)
	}
	host, ok := c.hosts[environment]
	if !ok {
		return fmt.Errorf("unknown APNs environment %q", environment)
	}
	bearer, err := s.bearer(time.Now())
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload(notification))
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+deviceToken, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("authorization", "bearer "+bearer)
	request.Header.Set("apns-topic", topic)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", "10")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("send to APNs: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}

	var failure struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
	switch {
	case response.StatusCode == http.StatusGone,
		failure.Reason == "BadDeviceToken",
		failure.Reason == "Unregistered",
		failure.Reason == "DeviceTokenNotForTopic":
		return fmt.Errorf("%w (%s)", ErrUnregistered, failure.Reason)
	case failure.Reason == "ExpiredProviderToken":
		s.reset()
	}
	return fmt.Errorf("APNs answered %d: %s", response.StatusCode, failure.Reason)
}

// payload builds the APNs JSON body.
func payload(notification Notification) map[string]any {
	alert := map[string]any{"title": notification.Title, "body": notification.Body}
	if notification.TitleLocKey != "" {
		alert["title-loc-key"] = notification.TitleLocKey
		alert["title-loc-args"] = nonNil(notification.TitleLocArgs)
	}
	if notification.BodyLocKey != "" {
		alert["loc-key"] = notification.BodyLocKey
		alert["loc-args"] = nonNil(notification.BodyLocArgs)
	}
	body := map[string]any{
		"aps": map[string]any{
			"alert":     alert,
			"sound":     "default",
			"thread-id": notification.ThreadID,
		},
	}
	for key, value := range notification.Data {
		if key != "aps" {
			body[key] = value
		}
	}
	return body
}

// nonNil turns a nil slice into an empty one, so JSON gets [] and not null.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// signer makes the JWT that authenticates the server to APNs. Apple refuses a
// token older than one hour and also refuses new tokens made too often, so one
// token is reused for 50 minutes.
type signer struct {
	teamID string
	keyID  string
	key    *ecdsa.PrivateKey

	mu       sync.Mutex
	token    string
	issuedAt time.Time
}

const tokenLifetime = 50 * time.Minute

func (s *signer) bearer(now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && now.Sub(s.issuedAt) < tokenLifetime {
		return s.token, nil
	}
	token, err := signES256(s.key, s.keyID, s.teamID, now)
	if err != nil {
		return "", err
	}
	s.token, s.issuedAt = token, now
	return token, nil
}

func (s *signer) reset() {
	s.mu.Lock()
	s.token = ""
	s.mu.Unlock()
}

// signES256 builds a compact JWT signed with ECDSA P-256 / SHA-256.
func signES256(key *ecdsa.PrivateKey, keyID, teamID string, now time.Time) (string, error) {
	encoding := base64.RawURLEncoding
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"iss": teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	unsigned := encoding.EncodeToString(header) + "." + encoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, sValue, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	// JWS wants r and s as two fixed 32-byte big-endian numbers.
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	sValue.FillBytes(signature[32:])
	return unsigned + "." + encoding.EncodeToString(signature), nil
}

// CheckCredential loads the .p8 file of a credential and signs a token with
// it, like a real push would, without sending anything to Apple.
func CheckCredential(credential Credential) error {
	pemData, err := os.ReadFile(credential.KeyFile)
	if err != nil {
		return fmt.Errorf("key %s: cannot read the .p8 file", credential.KeyID)
	}
	key, err := parseKey(pemData)
	if err != nil {
		return fmt.Errorf("key %s: %v", credential.KeyID, err)
	}
	if _, err := signES256(key, credential.KeyID, credential.TeamID, time.Now()); err != nil {
		return fmt.Errorf("key %s: cannot sign: %v", credential.KeyID, err)
	}
	return nil
}

// ValidateKey checks that the content of a .p8 file is an Apple APNs key.
func ValidateKey(pemData []byte) error {
	_, err := parseKey(pemData)
	return err
}

// parseKey reads an Apple .p8 file (a PKCS#8 PEM with a P-256 key).
func parseKey(pemData []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("no PEM data in key file")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an ECDSA key")
	}
	return key, nil
}
