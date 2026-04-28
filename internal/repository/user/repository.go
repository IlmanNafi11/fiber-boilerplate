package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	id := uuid.New().String()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, role, is_active)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, u.Email, u.PasswordHash, u.Role, u.IsActive,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505": // unique_violation
				return errx.ErrDuplicate
			case "23514", "23503": // check_violation, foreign_key_violation
				return errx.ErrConstraintViolation
			}
		}
		return err
	}

	u.ID = id
	return nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	u := &user.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, role, is_active, created_at, updated_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	u := &user.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, role, is_active, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}
