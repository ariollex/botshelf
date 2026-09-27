package maxauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidInitData = errors.New("MAX initData is invalid")
	ErrExpiredInitData = errors.New("MAX initData has expired")
)

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// Validate verifies MAX WebApp.initData according to the official two-stage
// HMAC-SHA256 algorithm. initDataUnsafe is never accepted here.
func Validate(raw, botToken string, now time.Time, maxAge time.Duration) (User, error) {
	if raw == "" || botToken == "" {
		return User{}, ErrInvalidInitData
	}
	pairs, err := parsePairs(raw)
	if err != nil {
		return User{}, err
	}
	receivedHash, ok := pairs["hash"]
	if !ok {
		return User{}, ErrInvalidInitData
	}
	delete(pairs, "hash")

	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+pairs[key])
	}

	first := hmac.New(sha256.New, []byte("WebAppData"))
	first.Write([]byte(botToken))
	second := hmac.New(sha256.New, first.Sum(nil))
	second.Write([]byte(strings.Join(lines, "\n")))
	expected := second.Sum(nil)
	provided, err := hex.DecodeString(receivedHash)
	if err != nil || subtle.ConstantTimeCompare(expected, provided) != 1 {
		return User{}, ErrInvalidInitData
	}

	authDate, err := strconv.ParseInt(pairs["auth_date"], 10, 64)
	if err != nil {
		return User{}, ErrInvalidInitData
	}
	issuedAt := time.Unix(authDate, 0)
	if now.Before(issuedAt.Add(-30*time.Second)) || now.Sub(issuedAt) > maxAge {
		return User{}, ErrExpiredInitData
	}

	var user User
	if err := json.Unmarshal([]byte(pairs["user"]), &user); err != nil || user.ID == 0 {
		return User{}, ErrInvalidInitData
	}
	return user, nil
}

func parsePairs(raw string) (map[string]string, error) {
	pairs := make(map[string]string)
	for _, part := range strings.Split(raw, "&") {
		key, value, found := strings.Cut(part, "=")
		if !found || key == "" {
			return nil, ErrInvalidInitData
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return nil, fmt.Errorf("%w: decode value", ErrInvalidInitData)
		}
		if _, duplicate := pairs[key]; duplicate {
			return nil, ErrInvalidInitData
		}
		pairs[key] = decoded
	}
	if pairs["auth_date"] == "" || pairs["user"] == "" {
		return nil, ErrInvalidInitData
	}
	return pairs, nil
}
