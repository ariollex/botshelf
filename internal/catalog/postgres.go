package catalog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func OpenPostgres(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	store := &PostgresStore{pool: pool}
	if err := store.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return store, nil
}

func (s *PostgresStore) Close() { s.pool.Close() }

func (s *PostgresStore) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *PostgresStore) ListBots(ctx context.Context, filter Filter) ([]Bot, error) {
	query := strings.TrimSpace(filter.Query)
	category := strings.TrimSpace(filter.Category)
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, name, description, category, source_url, last_verified_at
		FROM bots
		WHERE ($1 = '' OR category = $1)
		  AND ($2 = '' OR lower(name || ' ' || username || ' ' || description || ' ' || category) LIKE '%' || lower($2) || '%')
		ORDER BY last_verified_at DESC, name ASC`, category, query)
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	defer rows.Close()

	bots := make([]Bot, 0)
	for rows.Next() {
		bot, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		bots = append(bots, bot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bots: %w", err)
	}
	return bots, nil
}

func (s *PostgresStore) BotByID(ctx context.Context, id string) (Bot, error) {
	bot, err := scanBot(s.pool.QueryRow(ctx, `
		SELECT id, username, name, description, category, source_url, last_verified_at
		FROM bots WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Bot{}, ErrNotFound
	}
	if err != nil {
		return Bot{}, fmt.Errorf("get bot: %w", err)
	}
	return bot, nil
}

func (s *PostgresStore) CreateSubmission(ctx context.Context, input CreateSubmissionInput) (Submission, error) {
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return Submission{}, err
	}
	var alreadyPublished bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bots WHERE username = $1)`, input.Username).Scan(&alreadyPublished); err != nil {
		return Submission{}, fmt.Errorf("check existing bot: %w", err)
	}
	if alreadyPublished {
		return Submission{}, ErrAlreadyExists
	}

	submission := Submission{
		ID:           newID(),
		Username:     input.Username,
		Name:         input.Name,
		Description:  input.Description,
		Category:     input.Category,
		SourceURL:    input.SourceURL,
		OwnerContact: input.OwnerContact,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC(),
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO submissions (id, username, name, description, category, source_url, owner_contact, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		submission.ID, submission.Username, submission.Name, submission.Description, submission.Category,
		submission.SourceURL, submission.OwnerContact, submission.Status, submission.CreatedAt)
	if isUniqueViolation(err) {
		return Submission{}, ErrAlreadyExists
	}
	if err != nil {
		return Submission{}, fmt.Errorf("create submission: %w", err)
	}
	return submission, nil
}

func (s *PostgresStore) ListPendingSubmissions(ctx context.Context) ([]Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, name, description, category, source_url, owner_contact, status, created_at, reviewed_at, reviewer_id, moderator_note
		FROM submissions WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list pending submissions: %w", err)
	}
	defer rows.Close()

	items := make([]Submission, 0)
	for rows.Next() {
		item, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending submissions: %w", err)
	}
	return items, nil
}

func (s *PostgresStore) Approve(ctx context.Context, submissionID string, moderator Moderator) (Bot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Bot{}, fmt.Errorf("begin approval: %w", err)
	}
	defer tx.Rollback(ctx)

	submission, err := scanSubmission(tx.QueryRow(ctx, `
		SELECT id, username, name, description, category, source_url, owner_contact, status, created_at, reviewed_at, reviewer_id, moderator_note
		FROM submissions WHERE id = $1 FOR UPDATE`, submissionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Bot{}, ErrNotFound
	}
	if err != nil {
		return Bot{}, fmt.Errorf("load submission for approval: %w", err)
	}
	if submission.Status != StatusPending {
		return Bot{}, ErrNotPending
	}

	now := time.Now().UTC()
	bot := Bot{ID: newID(), Username: submission.Username, Name: submission.Name, Description: submission.Description, Category: submission.Category, SourceURL: submission.SourceURL, LastVerifiedAt: now}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bots (id, username, name, description, category, source_url, last_verified_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, bot.ID, bot.Username, bot.Name, bot.Description, bot.Category, bot.SourceURL, bot.LastVerifiedAt); err != nil {
		if isUniqueViolation(err) {
			return Bot{}, ErrAlreadyExists
		}
		return Bot{}, fmt.Errorf("publish bot: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE submissions SET status = 'verified', reviewed_at = $2, reviewer_id = $3, moderator_note = ''
		WHERE id = $1`, submissionID, now, moderator.ID); err != nil {
		return Bot{}, fmt.Errorf("mark approved submission: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Bot{}, fmt.Errorf("commit approval: %w", err)
	}
	return bot, nil
}

func (s *PostgresStore) Reject(ctx context.Context, submissionID string, moderator Moderator, note string) error {
	note = strings.TrimSpace(note)
	if len(note) < 3 || len(note) > 500 {
		return fmt.Errorf("%w: moderator note must contain 3-500 characters", ErrInvalidInput)
	}
	command, err := s.pool.Exec(ctx, `
		UPDATE submissions SET status = 'rejected', reviewed_at = NOW(), reviewer_id = $2, moderator_note = $3
		WHERE id = $1 AND status = 'pending'`, submissionID, moderator.ID, note)
	if err != nil {
		return fmt.Errorf("reject submission: %w", err)
	}
	if command.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submissions WHERE id = $1)`, submissionID).Scan(&exists); err != nil {
			return fmt.Errorf("check rejected submission: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
		return ErrNotPending
	}
	return nil
}

func (s *PostgresStore) CreateModeratorSession(ctx context.Context, rawToken string, moderator Moderator, expiresAt time.Time) error {
	hash := sha256.Sum256([]byte(rawToken))
	if _, err := s.pool.Exec(ctx, `DELETE FROM moderator_sessions WHERE expires_at <= NOW()`); err != nil {
		return fmt.Errorf("remove expired sessions: %w", err)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO moderator_sessions (token_hash, user_id, first_name, last_name, username, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, hash[:], moderator.ID, moderator.FirstName, moderator.LastName, moderator.Username, expiresAt)
	if err != nil {
		return fmt.Errorf("create moderator session: %w", err)
	}
	return nil
}

func (s *PostgresStore) ModeratorBySession(ctx context.Context, rawToken string) (Moderator, error) {
	hash := sha256.Sum256([]byte(rawToken))
	var moderator Moderator
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, first_name, last_name, username
		FROM moderator_sessions WHERE token_hash = $1 AND expires_at > NOW()`, hash[:]).Scan(
		&moderator.ID, &moderator.FirstName, &moderator.LastName, &moderator.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return Moderator{}, ErrUnauthorized
	}
	if err != nil {
		return Moderator{}, fmt.Errorf("load moderator session: %w", err)
	}
	return moderator, nil
}

type rowScanner interface{ Scan(...any) error }

func scanBot(row rowScanner) (Bot, error) {
	var bot Bot
	if err := row.Scan(&bot.ID, &bot.Username, &bot.Name, &bot.Description, &bot.Category, &bot.SourceURL, &bot.LastVerifiedAt); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

func scanSubmission(row rowScanner) (Submission, error) {
	var item Submission
	if err := row.Scan(&item.ID, &item.Username, &item.Name, &item.Description, &item.Category, &item.SourceURL, &item.OwnerContact, &item.Status, &item.CreatedAt, &item.ReviewedAt, &item.ReviewerID, &item.ModeratorNote); err != nil {
		return Submission{}, err
	}
	return item, nil
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}
