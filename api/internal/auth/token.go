package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// NewToken returns a random bearer token with 256 bits of entropy.
func NewToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// crypto/rand never fails on supported platforms; stop rather than
		// hand out a weak token.
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// HashToken returns the SHA-256 hex digest stored in the database. A stolen
// database dump then holds no usable token. A fast hash is fine here: the
// token is random, not a password.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// codeAlphabet has no look-alike characters (0/O, 1/I/L), so a code is easy to
// read out loud or type from a screenshot.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewCode returns a random human-friendly code, for example "K7PM-2XQR-9TWB".
func NewCode(groups int) string {
	const groupSize = 4
	bytes := make([]byte, groups*groupSize)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	var builder strings.Builder
	for i, b := range bytes {
		if i > 0 && i%groupSize == 0 {
			builder.WriteByte('-')
		}
		// 256 is not a multiple of 31: the bias is under 1%, fine for a
		// short-lived code that is also rate limited.
		builder.WriteByte(codeAlphabet[int(b)%len(codeAlphabet)])
	}
	return builder.String()
}

// NormalizeCode makes code input forgiving: case and dashes do not matter.
func NormalizeCode(code string) string {
	code = strings.ToUpper(code)
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, code)
}
