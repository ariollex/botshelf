package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/botshelf/internal/catalog"
)

func TestPendingSubmissionIsNotSearchableUntilApproved(t *testing.T) {
	store := &fakeStore{}
	submission, err := store.CreateSubmission(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreateSubmission: %v", err)
	}
	mux := http.NewServeMux()
	NewServer(store, nil, nil).Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/bots?q=%D1%81%D1%87%D1%91%D1%82", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"items\":[],\"total\":0}\n" {
		t.Fatalf("pending bot leaked into search: status=%d body=%s", response.Code, response.Body.String())
	}

	if _, err := store.Approve(context.Background(), submission.ID, catalog.Moderator{ID: 1}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() == "{\"items\":[],\"total\":0}\n" {
		t.Fatalf("approved bot missing from search: status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeStore struct {
	mu          sync.Mutex
	bots        []catalog.Bot
	submissions []catalog.Submission
}

func (s *fakeStore) ListBots(_ context.Context, filter catalog.Filter) ([]catalog.Bot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]catalog.Bot, 0)
	for _, bot := range s.bots {
		if filter.Query == "" || strings.Contains(bot.Name+" "+bot.Username+" "+bot.Description, filter.Query) {
			result = append(result, bot)
		}
	}
	return result, nil
}
func (s *fakeStore) BotByID(_ context.Context, id string) (catalog.Bot, error) {
	for _, bot := range s.bots {
		if bot.ID == id {
			return bot, nil
		}
	}
	return catalog.Bot{}, catalog.ErrNotFound
}
func (s *fakeStore) CreateSubmission(_ context.Context, input catalog.CreateSubmissionInput) (catalog.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := catalog.Submission{ID: "submission-1", Username: input.Username, Name: input.Name, Description: input.Description, Category: input.Category, SourceURL: input.SourceURL, OwnerContact: input.OwnerContact, Status: catalog.StatusPending, CreatedAt: time.Now()}
	s.submissions = append(s.submissions, item)
	return item, nil
}
func (s *fakeStore) ListPendingSubmissions(_ context.Context) ([]catalog.Submission, error) {
	return nil, nil
}
func (s *fakeStore) Approve(_ context.Context, id string, _ catalog.Moderator) (catalog.Bot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.submissions {
		if s.submissions[i].ID == id && s.submissions[i].Status == catalog.StatusPending {
			s.submissions[i].Status = catalog.StatusVerified
			bot := catalog.Bot{ID: "bot-1", Username: s.submissions[i].Username, Name: s.submissions[i].Name, Description: s.submissions[i].Description, Category: s.submissions[i].Category, SourceURL: s.submissions[i].SourceURL, LastVerifiedAt: time.Now()}
			s.bots = append(s.bots, bot)
			return bot, nil
		}
	}
	return catalog.Bot{}, catalog.ErrNotFound
}
func (s *fakeStore) Reject(context.Context, string, catalog.Moderator, string) error { return nil }
func (s *fakeStore) CreateModeratorSession(context.Context, string, catalog.Moderator, time.Time) error {
	return nil
}
func (s *fakeStore) ModeratorBySession(context.Context, string) (catalog.Moderator, error) {
	return catalog.Moderator{}, catalog.ErrUnauthorized
}
func validInput() catalog.CreateSubmissionInput {
	return catalog.CreateSubmissionInput{Username: "invoice_helper_bot", Name: "Помощник по счетам", Description: "Помогает предпринимателю подготовить счёт и проверить обязательные реквизиты перед отправкой клиенту.", Category: "Финансы", SourceURL: "https://example.org/invoice-helper", OwnerContact: "owner@example.org"}
}
