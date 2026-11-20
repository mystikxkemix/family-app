package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if ok, err := CheckPassword("correct horse battery", hash); err != nil || !ok {
		t.Fatalf("right password refused: ok=%v err=%v", ok, err)
	}
	if ok, _ := CheckPassword("wrong", hash); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := CheckPassword("x", "$bcrypt$nope"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}

func TestHashesAreSalted(t *testing.T) {
	first, _ := HashPassword("same")
	second, _ := HashPassword("same")
	if first == second {
		t.Fatal("two hashes of the same password are equal: salt missing")
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) < 40 {
		t.Fatalf("weak tokens: %q %q", a, b)
	}
	if HashToken(a) == a || len(HashToken(a)) != 64 {
		t.Fatal("token hash is not a SHA-256 hex digest")
	}
}

func TestCodes(t *testing.T) {
	code := NewCode(3)
	if len(code) != 14 || strings.Count(code, "-") != 2 {
		t.Fatalf("unexpected code shape: %q", code)
	}
	if NormalizeCode(" k7pm-2xqr 9twb ") != "K7PM2XQR9TWB" {
		t.Fatal("NormalizeCode must ignore case, dashes and spaces")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewLimiter(60, 2) // one token per second, burst 2
	limiter.now = func() time.Time { return now }

	if !limiter.Allow("a") || !limiter.Allow("a") {
		t.Fatal("burst refused")
	}
	if limiter.Allow("a") {
		t.Fatal("third request allowed with an empty bucket")
	}
	if !limiter.Allow("b") {
		t.Fatal("keys must not share a bucket")
	}
	now = now.Add(time.Second)
	if !limiter.Allow("a") {
		t.Fatal("token not refilled after one second")
	}
	now = now.Add(time.Hour)
	limiter.Allow("c") // triggers a sweep
	if _, ok := limiter.buckets["a"]; ok {
		t.Fatal("idle full bucket not swept")
	}
}
