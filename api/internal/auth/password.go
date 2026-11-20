// Package auth holds the security building blocks: password hashing, random
// tokens and rate limiting.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id settings from the OWASP password storage cheat sheet (19 MiB, 2
// passes). They stay cheap enough for a small home server.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLength = 32
	argonSaltBytes = 16
)

var errBadHash = errors.New("invalid password hash")

// HashPassword returns an Argon2id hash in the PHC string format:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLength)
	encoding := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		encoding.EncodeToString(salt), encoding.EncodeToString(key)), nil
}

// CheckPassword reports whether the password matches the hash. The parameters
// come from the hash, so old hashes keep working if the settings change.
func CheckPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false, errBadHash
	}
	encoding := base64.RawStdEncoding
	salt, err := encoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	expected, err := encoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	actual := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// dummyHash is checked when the username does not exist, so a failed login
// takes the same time whether or not the account exists.
var dummyHash, _ = HashPassword("not-a-real-password")

// BurnPasswordCheck spends the same time as CheckPassword, with no result.
func BurnPasswordCheck(password string) {
	_, _ = CheckPassword(password, dummyHash)
}
