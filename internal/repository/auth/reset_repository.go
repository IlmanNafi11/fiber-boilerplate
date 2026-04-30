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

type PasswordResetTokenRepository struct {
	pool *pgxpool.Pool
}

func NewPasswordResetTokenRepository(pool *pgxpool.Pool) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{pool: pool}
}

func (r *PasswordResetTokenRepository) Create(ctx context.Context, t *auth.PasswordResetToken) error {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt,
	)
	return err
}

func (r *PasswordResetTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*auth.PasswordResetToken, error) {
	t := &auth.PasswordResetToken{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at, created_at
		 FROM password_reset_tokens WHERE token_hash = $1`,
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

func (r *PasswordResetTokenRepository) MarkUsed(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
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

func (r *PasswordResetTokenRepository) MarkUsedWithTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
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

func (r *PasswordResetTokenRepository) MarkUsedByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	)
	return err
}
