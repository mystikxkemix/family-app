package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// apiError is an error with an HTTP status and a stable code the app can test.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func badRequest(format string, args ...any) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: fmt.Sprintf(format, args...)}
}

var (
	errNotFound     = &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
	errUnauthorized = &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "login required"}
	errRateLimited  = &apiError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "too many requests, try again later"}
)

// handlerFunc is an HTTP handler that returns its error instead of writing it.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// noContent answers 204 with no body.
func noContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// writeError maps an error to a JSON error response. Unknown errors are
// logged by the caller and shown as a generic 500, never with their details.
func writeError(w http.ResponseWriter, err error) (internal bool) {
	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr):
	case errors.Is(err, store.ErrNotFound):
		apiErr = errNotFound
	case errors.Is(err, store.ErrConflict):
		apiErr = &apiError{Status: http.StatusConflict, Code: "conflict", Message: "already exists"}
	default:
		apiErr = &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"}
		internal = true
	}
	writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
	return internal
}

// decodeJSON reads a JSON body into value. Unknown fields are refused so a
// typo in the app shows up as an error instead of being ignored.
func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Message: "request body too large"}
		}
		return badRequest("invalid JSON body: %v", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return badRequest("invalid JSON body: extra data after the object")
	}
	return nil
}

// pathID reads an ObjectID from a path parameter.
func pathID(r *http.Request, name string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(r.PathValue(name))
	if err != nil {
		return bson.ObjectID{}, errNotFound
	}
	return id, nil
}

// cleanText trims a text field and checks its length (in characters).
func cleanText(field, value string, required bool, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", badRequest("%s is required", field)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return "", badRequest("%s is too long (max %d characters)", field, maxLength)
	}
	return value, nil
}

// oneOf checks that a value is one of the allowed ones.
func oneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return badRequest("%s must be one of: %s", field, strings.Join(allowed, ", "))
}
