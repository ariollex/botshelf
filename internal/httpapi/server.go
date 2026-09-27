package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/example/botshelf/internal/catalog"
	"github.com/example/botshelf/internal/maxauth"
)

type readinessChecker interface {
	Ping(context.Context) error
}

type Server struct {
	store catalog.Store
	auth  *maxauth.Service
	ready readinessChecker
}

func NewServer(store catalog.Store, auth *maxauth.Service, ready readinessChecker) *Server {
	return &Server{store: store, auth: auth, ready: ready}
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /api/v1/categories", s.handleCategories)
	mux.HandleFunc("GET /api/v1/bots", s.handleBots)
	mux.HandleFunc("GET /api/v1/bots/", s.handleBotByID)
	mux.HandleFunc("POST /api/v1/submissions", s.handleCreateSubmission)
	mux.HandleFunc("POST /api/v1/auth/max", s.handleMaxLogin)
	mux.HandleFunc("POST /api/v1/auth/max/identity", s.handleMaxIdentity)
	mux.HandleFunc("GET /api/v1/admin/submissions", s.handlePendingSubmissions)
	mux.HandleFunc("POST /api/v1/admin/submissions/", s.handleModerationAction)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.ready == nil || s.ready.Ping(r.Context()) != nil {
		writeError(w, http.StatusServiceUnavailable, "service is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleCategories(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"categories": catalog.Categories})
}

func (s *Server) handleBots(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 128 {
		writeError(w, http.StatusBadRequest, "search query is too long")
		return
	}
	bots, err := s.store.ListBots(r.Context(), catalog.Filter{Query: query, Category: r.URL.Query().Get("category")})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not search catalog")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": bots, "total": len(bots)})
}

func (s *Server) handleBotByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/bots/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "bot not found")
		return
	}
	bot, err := s.store.BotByID(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bot")
		return
	}
	writeJSON(w, http.StatusOK, bot)
}

func (s *Server) handleCreateSubmission(w http.ResponseWriter, r *http.Request) {
	var input catalog.CreateSubmissionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	submission, err := s.store.CreateSubmission(r.Context(), input)
	if errors.Is(err, catalog.ErrInvalidInput) || errors.Is(err, catalog.ErrAlreadyExists) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save submission")
		return
	}
	writeJSON(w, http.StatusCreated, submission)
}

func (s *Server) handleMaxLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "moderator authentication is not configured")
		return
	}
	var request struct {
		InitData string `json:"init_data"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	token, expiresAt, err := s.auth.Login(r.Context(), request.InitData)
	if errors.Is(err, maxauth.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "moderator authentication is not configured")
		return
	}
	if errors.Is(err, maxauth.ErrForbidden) {
		writeError(w, http.StatusForbidden, "this MAX account is not a moderator")
		return
	}
	if errors.Is(err, maxauth.ErrInvalidInitData) || errors.Is(err, maxauth.ErrExpiredInitData) {
		writeError(w, http.StatusUnauthorized, "MAX authorization data is invalid or expired")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create moderator session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": expiresAt})
}

func (s *Server) handleMaxIdentity(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "moderator authentication is not configured")
		return
	}
	var request struct {
		InitData string `json:"init_data"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	user, err := s.auth.Identity(request.InitData)
	if errors.Is(err, maxauth.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "moderator authentication is not configured")
		return
	}
	if errors.Is(err, maxauth.ErrInvalidInitData) || errors.Is(err, maxauth.ErrExpiredInitData) {
		writeError(w, http.StatusUnauthorized, "MAX authorization data is invalid or expired")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not verify MAX identity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"user_id": user.ID})
}

func (s *Server) handlePendingSubmissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireModerator(w, r); !ok {
		return
	}
	items, err := s.store.ListPendingSubmissions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load moderation queue")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleModerationAction(w http.ResponseWriter, r *http.Request) {
	moderator, ok := s.requireModerator(w, r)
	if !ok {
		return
	}
	const prefix = "/api/v1/admin/submissions/"
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) != 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "unknown moderator action")
		return
	}

	var err error
	switch parts[1] {
	case "approve":
		bot, approvalErr := s.store.Approve(r.Context(), parts[0], moderator)
		if approvalErr == nil {
			writeJSON(w, http.StatusOK, bot)
			return
		}
		err = approvalErr
	case "reject":
		var request struct {
			Note string `json:"note"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		err = s.store.Reject(r.Context(), parts[0], moderator, request.Note)
		if err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	default:
		writeError(w, http.StatusNotFound, "unknown moderator action")
		return
	}

	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "submission not found")
		return
	}
	if errors.Is(err, catalog.ErrNotPending) || errors.Is(err, catalog.ErrAlreadyExists) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, catalog.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "moderation action failed")
}

func (s *Server) requireModerator(w http.ResponseWriter, r *http.Request) (catalog.Moderator, bool) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "moderator authentication is not configured")
		return catalog.Moderator{}, false
	}
	moderator, err := s.auth.Moderator(r.Context(), r.Header.Get("Authorization"))
	if errors.Is(err, catalog.ErrUnauthorized) {
		writeError(w, http.StatusUnauthorized, "moderator authorization is required")
		return catalog.Moderator{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check moderator authorization")
		return catalog.Moderator{}, false
	}
	return moderator, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("write JSON response: %v\n", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
