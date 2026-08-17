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

type EmailVerificationTokenRepository struct {
	pool *pgxpool.Pool
}

func NewEmailVerificationTokenRepository(pool *pgxpool.Pool) *EmailVerificationTokenRepository {
	return &EmailVerificationTokenRepository{pool: pool}
}

func (r *EmailVerificationTokenRepository) Create(ctx context.Context, t *auth.EmailVerificationToken) error {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt,
	)
	return err
}

func (r *EmailVerificationTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*auth.EmailVerificationToken, error) {
	t := &auth.EmailVerificationToken{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at, created_at
		 FROM email_verification_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errx.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *EmailVerificationTokenRepository) MarkUsed(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
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

func (r *EmailVerificationTokenRepository) MarkUsedByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	)
	return err
}
