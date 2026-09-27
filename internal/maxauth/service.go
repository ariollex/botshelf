package maxauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/botshelf/internal/catalog"
)

var (
	ErrForbidden     = errors.New("this MAX user is not a moderator")
	ErrNotConfigured = errors.New("MAX moderator authentication is not configured")
)

type Service struct {
	store       catalog.Store
	botToken    string
	moderatorID map[int64]struct{}
	sessionTTL  time.Duration
	initDataAge time.Duration
	now         func() time.Time
}

func New(store catalog.Store, botToken string, moderatorIDs []int64) *Service {
	allowed := make(map[int64]struct{}, len(moderatorIDs))
	for _, id := range moderatorIDs {
		allowed[id] = struct{}{}
	}
	return &Service{store: store, botToken: botToken, moderatorID: allowed, sessionTTL: 8 * time.Hour, initDataAge: 10 * time.Minute, now: time.Now}
}

func (s *Service) Login(ctx context.Context, initData string) (string, time.Time, error) {
	if s.botToken == "" || len(s.moderatorID) == 0 {
		return "", time.Time{}, ErrNotConfigured
	}
	user, err := Validate(initData, s.botToken, s.now(), s.initDataAge)
	if err != nil {
		return "", time.Time{}, err
	}
	if _, allowed := s.moderatorID[user.ID]; !allowed {
		return "", time.Time{}, ErrForbidden
	}

	token, err := newSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	moderator := catalog.Moderator{ID: user.ID, FirstName: user.FirstName, LastName: user.LastName, Username: user.Username}
	if err := s.store.CreateModeratorSession(ctx, token, moderator, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// Identity returns the MAX user encoded in signed launch data. It deliberately
// does not grant any role and is used only to help a future moderator discover
// their own numeric MAX ID during initial configuration.
func (s *Service) Identity(initData string) (User, error) {
	if s.botToken == "" {
		return User{}, ErrNotConfigured
	}
	return Validate(initData, s.botToken, s.now(), s.initDataAge)
}

func (s *Service) Moderator(ctx context.Context, authorization string) (catalog.Moderator, error) {
	token, found := strings.CutPrefix(authorization, "Bearer ")
	if !found || token == "" {
		return catalog.Moderator{}, catalog.ErrUnauthorized
	}
	return s.store.ModeratorBySession(ctx, token)
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
