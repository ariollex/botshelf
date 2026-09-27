package maxauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateAcceptsCorrectlySignedData(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	raw := signedInitData("test-token", map[string]string{
		"auth_date": "1790424000",
		"query_id":  "test-query",
		"user":      `{"id":42,"first_name":"Ира","last_name":"Тест","username":"ira"}`,
	})
	user, err := Validate(raw, "test-token", now, 10*time.Minute)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if user.ID != 42 || user.Username != "ira" {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestValidateRejectsChangedUser(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	raw := signedInitData("test-token", map[string]string{
		"auth_date": "1790424000",
		"user":      `{"id":42,"first_name":"Ира"}`,
	})
	tampered := strings.Replace(raw, "%22id%22%3A42", "%22id%22%3A43", 1)
	if _, err := Validate(tampered, "test-token", now, 10*time.Minute); err != ErrInvalidInitData {
		t.Fatalf("error=%v, want invalid data", err)
	}
}

func TestValidateRejectsExpiredData(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 20, 0, 0, time.UTC)
	raw := signedInitData("test-token", map[string]string{
		"auth_date": "1790424000",
		"user":      `{"id":42,"first_name":"Ира"}`,
	})
	if _, err := Validate(raw, "test-token", now, 10*time.Minute); err != ErrExpiredInitData {
		t.Fatalf("error=%v, want expired data", err)
	}
}

func TestIdentityReturnsVerifiedUserWithoutModeratorRole(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	service := New(nil, "test-token", nil)
	service.now = func() time.Time { return now }
	raw := signedInitData("test-token", map[string]string{
		"auth_date": strconv.FormatInt(now.Unix(), 10),
		"user":      `{"id":42,"first_name":"Ира"}`,
	})

	user, err := service.Identity(raw)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if user.ID != 42 {
		t.Fatalf("user ID = %d, want 42", user.ID)
	}
}

func signedInitData(botToken string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+params[key])
	}
	first := hmac.New(sha256.New, []byte("WebAppData"))
	first.Write([]byte(botToken))
	second := hmac.New(sha256.New, first.Sum(nil))
	second.Write([]byte(strings.Join(lines, "\n")))
	params["hash"] = hex.EncodeToString(second.Sum(nil))

	keys = keys[:0]
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoded := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded = append(encoded, key+"="+url.QueryEscape(params[key]))
	}
	return strings.Join(encoded, "&")
}
