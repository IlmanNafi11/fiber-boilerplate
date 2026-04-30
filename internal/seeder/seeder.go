package seeder

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
)

func Run(ctx context.Context, pool *pgxpool.Pool, cfg config.SeederConfig, env string, force bool, logger *zap.Logger) error {
	if env == "production" && !force {
		return fmt.Errorf("seeder refused to run in production environment. Use --force flag to override")
	}

	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return fmt.Errorf("ADMIN_EMAIL and ADMIN_PASSWORD environment variables are required")
	}

	if err := validatePassword(cfg.AdminPassword); err != nil {
		return fmt.Errorf("ADMIN_PASSWORD invalid: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			// Expected "already committed" after successful commit
		}
	}()

	createdUsers := 0
	skippedUsers := 0

	// Seed admin user
	adminID, wasCreated, err := seedAdmin(ctx, tx, cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to seed admin user: %w", err)
	}
	if wasCreated {
		createdUsers++
	} else {
		skippedUsers++
	}

	// Seed demo users
	userIDs := []string{adminID}

	if cfg.DemoEmail != "" && cfg.DemoPassword != "" {
		demoIDs, demoCreated, demoSkipped, err := seedDemoUsers(ctx, tx, cfg, logger)
		if err != nil {
			return fmt.Errorf("failed to seed demo users: %w", err)
		}
		userIDs = append(userIDs, demoIDs...)
		createdUsers += demoCreated
		skippedUsers += demoSkipped
	} else {
		logger.Info("DEMO_EMAIL not set, skipping demo users")
	}

	// Seed products
	createdProducts, skippedProducts, err := seedProducts(ctx, tx, userIDs, logger)
	if err != nil {
		return fmt.Errorf("failed to seed products: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("Seed complete",
		zap.Int("users_created", createdUsers),
		zap.Int("users_skipped", skippedUsers),
		zap.Int("products_created", createdProducts),
		zap.Int("products_skipped", skippedProducts),
	)

	return nil
}

func seedAdmin(ctx context.Context, tx pgx.Tx, cfg config.SeederConfig, logger *zap.Logger) (string, bool, error) {
	hash, err := hashPassword(cfg.AdminPassword)
	if err != nil {
		return "", false, err
	}

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role, is_active, email_verified_at)
		 VALUES ($1, $2, 'admin', true, NOW())
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id`,
		cfg.AdminEmail, hash,
	).Scan(&id)

	if err != nil {
		// pgx returns pgx.ErrNoRows when ON CONFLICT DO NOTHING and no row returned
		logger.Info("Skipped admin user (already exists)", zap.String("email", cfg.AdminEmail))
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, cfg.AdminEmail).Scan(&id)
		return id, false, err
	}

	logger.Info("Created admin user", zap.String("email", cfg.AdminEmail))
	return id, true, nil
}

func seedDemoUsers(ctx context.Context, tx pgx.Tx, cfg config.SeederConfig, logger *zap.Logger) ([]string, int, int, error) {
	baseEmail := cfg.DemoEmail
	atIdx := strings.Index(baseEmail, "@")
	if atIdx <= 0 || atIdx >= len(baseEmail)-1 {
		return nil, 0, 0, fmt.Errorf("DEMO_EMAIL %q is not a valid email address", baseEmail)
	}
	localPart := baseEmail[:atIdx]
	domain := baseEmail[atIdx+1:]

	demoEmails := []string{
		baseEmail,
		localPart + "1@" + domain,
		localPart + "2@" + domain,
	}

	var ids []string
	created := 0
	skipped := 0

	hash, err := hashPassword(cfg.DemoPassword)
	if err != nil {
		return nil, 0, 0, err
	}

	for _, email := range demoEmails {
		if email == cfg.AdminEmail {
			logger.Warn("Demo user collides with admin, skipping", zap.String("email", email))
			continue
		}

		var id string
		err := tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, role, is_active, email_verified_at)
			 VALUES ($1, $2, 'user', true, NOW())
			 ON CONFLICT (email) DO NOTHING
			 RETURNING id`,
			email, hash,
		).Scan(&id)

		if err != nil {
			// Already exists
			logger.Info("Skipped demo user (already exists)", zap.String("email", email))
			if scanErr := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id); scanErr != nil {
				return nil, 0, 0, scanErr
			}
			ids = append(ids, id)
			skipped++
			continue
		}

		logger.Info("Created demo user", zap.String("email", email))
		ids = append(ids, id)
		created++
	}

	return ids, created, skipped, nil
}

func seedProducts(ctx context.Context, tx pgx.Tx, userIDs []string, logger *zap.Logger) (int, int, error) {
	products := []struct {
		Name        string
		Description string
		Price       float64
	}{
		{"Wireless Mouse", "Ergonomic wireless mouse with USB receiver", 29.99},
		{"Mechanical Keyboard", "Cherry MX Blue switches, RGB backlit", 89.99},
		{"USB-C Hub", "7-in-1 USB-C hub with HDMI, USB 3.0, SD card", 49.99},
		{"Webcam HD", "1080p HD webcam with built-in microphone", 59.99},
		{"Laptop Stand", "Adjustable aluminum laptop stand", 39.99},
	}

	created := 0
	skipped := 0

	for i, p := range products {
		ownerID := userIDs[i%len(userIDs)]

		var existingID string
		err := tx.QueryRow(ctx,
			`SELECT id FROM products WHERE name = $1 AND user_id = $2 AND deleted_at IS NULL`,
			p.Name, ownerID,
		).Scan(&existingID)

		if err == nil {
			logger.Info("Skipped product (already exists)", zap.String("name", p.Name))
			skipped++
			continue
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO products (user_id, name, description, price)
			 VALUES ($1, $2, $3, $4)`,
			ownerID, p.Name, p.Description, p.Price,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to insert product %q: %w", p.Name, err)
		}

		logger.Info("Created product", zap.String("name", p.Name), zap.String("owner", ownerID))
		created++
	}

	return created, skipped, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("must be at least 8 characters")
	}
	var hasLetter, hasDigit bool
	for _, c := range password {
		if unicode.IsLetter(c) {
			hasLetter = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}
	if !hasLetter {
		return fmt.Errorf("must contain at least 1 letter")
	}
	if !hasDigit {
		return fmt.Errorf("must contain at least 1 digit")
	}
	return nil
}
