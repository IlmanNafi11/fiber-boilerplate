package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- SessionRepository ---

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *auth.Session) error {
	s.ID = uuid.New().String()
	s.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, s.UserAgent, s.IPAddress, s.ExpiresAt,
	)
	return err
}

func (r *SessionRepository) GetByID(ctx context.Context, id string) (*auth.Session, error) {
	s := &auth.Session{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, user_agent, ip_address, revoked_at, created_at, expires_at
		 FROM sessions WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.UserID, &s.UserAgent, &s.IPAddress, &s.RevokedAt, &s.CreatedAt, &s.ExpiresAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *SessionRepository) RevokeByID(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errx.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) RevokeByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

// --- RefreshTokenRepository ---

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *auth.RefreshToken) error {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (id, session_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		t.ID, t.SessionID, t.TokenHash, t.ExpiresAt,
	)
	return err
}

func (r *RefreshTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*auth.RefreshToken, error) {
	t := &auth.RefreshToken{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, session_id, token_hash, revoked_at, grace_until, created_at, expires_at
		 FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.SessionID, &t.TokenHash, &t.RevokedAt, &t.GraceUntil, &t.CreatedAt, &t.ExpiresAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *RefreshTokenRepository) RevokeByID(ctx context.Context, id string, graceUntil *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW(), grace_until = $1 WHERE id = $2`,
		graceUntil, id,
	)
	return err
}

func (r *RefreshTokenRepository) RevokeBySessionID(ctx context.Context, sessionID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE session_id = $1 AND revoked_at IS NULL`,
		sessionID,
	)
	return err
}

func (r *RefreshTokenRepository) CreateWithTx(ctx context.Context, tx pgx.Tx, t *auth.RefreshToken) error {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()

	_, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (id, session_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		t.ID, t.SessionID, t.TokenHash, t.ExpiresAt,
	)
	return err
}

func (r *RefreshTokenRepository) RevokeWithTx(ctx context.Context, tx pgx.Tx, id string, graceUntil *time.Time) error {
	_, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW(), grace_until = $1 WHERE id = $2`,
		graceUntil, id,
	)
	return err
}

func (r *RefreshTokenRepository) GetBySessionIDNewest(ctx context.Context, sessionID string) (*auth.RefreshToken, error) {
	t := &auth.RefreshToken{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, session_id, token_hash, revoked_at, grace_until, created_at, expires_at
		 FROM refresh_tokens
		 WHERE session_id = $1 AND revoked_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`,
		sessionID,
	).Scan(&t.ID, &t.SessionID, &t.TokenHash, &t.RevokedAt, &t.GraceUntil, &t.CreatedAt, &t.ExpiresAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}
