package catalog

import (
	"context"
	"time"
)

// Status is deliberately small: a bot is either visible after human
// verification, waiting for it, or rejected with a moderator note.
type Status string

const (
	StatusPending  Status = "pending"
	StatusVerified Status = "verified"
	StatusRejected Status = "rejected"
)

var Categories = []string{
	"Продажи",
	"Клиенты",
	"Документы",
	"Финансы",
	"Команда",
	"Аналитика",
}

type Bot struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Category       string    `json:"category"`
	SourceURL      string    `json:"source_url"`
	LastVerifiedAt time.Time `json:"last_verified_at"`
}

// Submission is kept separately from Bot. A pending submission must never
// appear in search results; otherwise anyone could publish a deceptive link.
type Submission struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Category      string     `json:"category"`
	SourceURL     string     `json:"source_url"`
	OwnerContact  string     `json:"owner_contact"`
	Status        Status     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID    *int64     `json:"reviewer_id,omitempty"`
	ModeratorNote string     `json:"moderator_note,omitempty"`
}

type Filter struct {
	Query    string
	Category string
}

type CreateSubmissionInput struct {
	Username     string `json:"username"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Category     string `json:"category"`
	SourceURL    string `json:"source_url"`
	OwnerContact string `json:"owner_contact"`
}

type Moderator struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
}

// Store is deliberately independent from PostgreSQL. The HTTP layer is only
// interested in catalog operations, so a test double can be used in unit
// tests and a different database can be used later without changing handlers.
type Store interface {
	ListBots(context.Context, Filter) ([]Bot, error)
	BotByID(context.Context, string) (Bot, error)
	CreateSubmission(context.Context, CreateSubmissionInput) (Submission, error)
	ListPendingSubmissions(context.Context) ([]Submission, error)
	Approve(context.Context, string, Moderator) (Bot, error)
	Reject(context.Context, string, Moderator, string) error
	CreateModeratorSession(context.Context, string, Moderator, time.Time) error
	ModeratorBySession(context.Context, string) (Moderator, error)
}
