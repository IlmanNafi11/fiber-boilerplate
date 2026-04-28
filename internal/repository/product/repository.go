package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/product"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

// Create inserts a new product and returns it with generated fields populated.
func (r *ProductRepository) Create(ctx context.Context, p *product.Product) error {
	id := uuid.New().String()
	err := r.pool.QueryRow(ctx,
		`INSERT INTO products (id, user_id, name, description, price)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING created_at, updated_at`,
		id, p.UserID, p.Name, p.Description, p.Price,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return errx.ErrDuplicate
			case "23514", "23503":
				return errx.ErrConstraintViolation
			}
		}
		return err
	}
	p.ID = id
	return nil
}

// GetByID fetches a single active (non-deleted) product by ID with user email via JOIN.
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*product.Product, string, error) {
	p := &product.Product{}
	var userEmail string
	err := r.pool.QueryRow(ctx,
		`SELECT p.id, p.user_id, p.name, p.description, p.price,
		        p.created_at, p.updated_at, u.email
		 FROM products p JOIN users u ON p.user_id = u.id
		 WHERE p.id = $1 AND p.deleted_at IS NULL`,
		id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.Price,
		&p.CreatedAt, &p.UpdatedAt, &userEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", product.ErrProductNotFound
		}
		return nil, "", err
	}
	return p, userEmail, nil
}

// List returns a paginated list of active products with user emails.
func (r *ProductRepository) List(ctx context.Context, page, limit int) ([]*product.Product, []string, int, error) {
	// Count total active products
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM products WHERE deleted_at IS NULL`,
	).Scan(&total); err != nil {
		return nil, nil, 0, err
	}

	offset := (page - 1) * limit
	rows, err := r.pool.Query(ctx,
		`SELECT p.id, p.user_id, p.name, p.description, p.price,
		        p.created_at, p.updated_at, u.email
		 FROM products p JOIN users u ON p.user_id = u.id
		 WHERE p.deleted_at IS NULL
		 ORDER BY p.created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()

	var products []*product.Product
	var userEmails []string
	for rows.Next() {
		p := &product.Product{}
		var email string
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.Price,
			&p.CreatedAt, &p.UpdatedAt, &email); err != nil {
			return nil, nil, 0, err
		}
		products = append(products, p)
		userEmails = append(userEmails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, err
	}

	return products, userEmails, total, nil
}

// Update performs a partial update on a product using dynamic SQL.
// Only non-nil fields from the UpdateProductRequest are updated.
func (r *ProductRepository) Update(ctx context.Context, id string, req *product.UpdateProductRequest) (*product.Product, string, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Price != nil {
		setClauses = append(setClauses, fmt.Sprintf("price = $%d", argIdx))
		args = append(args, *req.Price)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil, "", errx.BadRequest("no fields to update")
	}

	// Always update updated_at (application-level, D-27)
	setClauses = append(setClauses, "updated_at = NOW()")

	// Add WHERE clause parameter
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE products SET %s WHERE id = $%d AND deleted_at IS NULL
		 RETURNING user_id, name, description, price, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx,
	)

	p := &product.Product{ID: id}
	var userEmail string
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.UserID, &p.Name, &p.Description, &p.Price,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", product.ErrProductNotFound
		}
		return nil, "", err
	}

	// Fetch user email separately (simpler than dynamic SQL with JOIN)
	err = r.pool.QueryRow(ctx,
		`SELECT email FROM users WHERE id = $1`, p.UserID,
	).Scan(&userEmail)
	if err != nil {
		return nil, "", err
	}

	return p, userEmail, nil
}

// SoftDelete sets deleted_at on a product. Returns the product data for response.
// Idempotent: returns the product even if already soft-deleted (D-22).
func (r *ProductRepository) SoftDelete(ctx context.Context, id string) (*product.Product, string, error) {
	// First fetch the product (including soft-deleted for idempotent response)
	p := &product.Product{}
	var userEmail string
	err := r.pool.QueryRow(ctx,
		`SELECT p.id, p.user_id, p.name, p.description, p.price,
		        p.created_at, p.updated_at, p.deleted_at, u.email
		 FROM products p JOIN users u ON p.user_id = u.id
		 WHERE p.id = $1`,
		id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.Price,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt, &userEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", product.ErrProductNotFound
		}
		return nil, "", err
	}

	// Only set deleted_at if not already soft-deleted
	if p.DeletedAt == nil {
		_, err = r.pool.Exec(ctx,
			`UPDATE products SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
			id,
		)
		if err != nil {
			return nil, "", err
		}
	}

	return p, userEmail, nil
}
