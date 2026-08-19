// Package repository provides durable persistence for the email outbox.
package repository

import (
	"context"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists and dispatches email outbox events.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs an outbox repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// EnqueueWithTx inserts a pending outbox event within the caller's transaction,
// so the event commits atomically with the auth state change that produced it.
// On success the event's server-generated fields are populated.
func (r *Repository) EnqueueWithTx(ctx context.Context, tx pgx.Tx, e *email.OutboxEvent) error {
	return tx.QueryRow(ctx,
		`INSERT INTO email_outbox (event_type, recipient, payload)
		 VALUES ($1, $2, $3)
		 RETURNING id, status, attempts, next_attempt_at, created_at, updated_at`,
		e.EventType, e.Recipient, e.Payload,
	).Scan(&e.ID, &e.Status, &e.Attempts, &e.NextAttemptAt, &e.CreatedAt, &e.UpdatedAt)
}

// ClaimBatch atomically leases up to limit due events to workerID. An event is
// claimable when it is pending, its next_attempt_at has arrived, and it is
// either unlocked or its lease has expired (locked_at older than now-lease).
// FOR UPDATE SKIP LOCKED guarantees two concurrent workers never claim the same
// row. The returned events carry the fresh lease.
func (r *Repository) ClaimBatch(ctx context.Context, workerID string, limit int, lease time.Duration, now time.Time) ([]*email.OutboxEvent, error) {
	leaseCutoff := now.Add(-lease)

	rows, err := r.pool.Query(ctx,
		`UPDATE email_outbox SET
			locked_at = $1,
			locked_by = $2,
			updated_at = $1
		 WHERE id IN (
			SELECT id FROM email_outbox
			WHERE status = 'pending'
			  AND next_attempt_at <= $1
			  AND (locked_at IS NULL OR locked_at < $3)
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $4
		 )
		 RETURNING id, event_type, recipient, payload, status, attempts,
		           next_attempt_at, locked_at, locked_by, last_error, created_at, updated_at`,
		now, workerID, leaseCutoff, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*email.OutboxEvent
	for rows.Next() {
		e := &email.OutboxEvent{}
		if err := rows.Scan(
			&e.ID, &e.EventType, &e.Recipient, &e.Payload, &e.Status, &e.Attempts,
			&e.NextAttemptAt, &e.LockedAt, &e.LockedBy, &e.LastError, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkSent marks a claimed event as delivered and releases its lease.
func (r *Repository) MarkSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE email_outbox SET
			status = 'sent',
			locked_at = NULL,
			locked_by = NULL,
			updated_at = NOW()
		 WHERE id = $1`,
		id,
	)
	return err
}

// MarkRetry records a failed attempt, releases the lease, and either reschedules
// the event (dead=false) or dead-letters it (dead=true). The attempt counter is
// incremented and the sanitized error is stored for diagnostics.
func (r *Repository) MarkRetry(ctx context.Context, id string, nextAttemptAt time.Time, lastError string, dead bool) error {
	status := email.OutboxStatusPending
	if dead {
		status = email.OutboxStatusDead
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE email_outbox SET
			status = $2,
			attempts = attempts + 1,
			next_attempt_at = $3,
			last_error = $4,
			locked_at = NULL,
			locked_by = NULL,
			updated_at = NOW()
		 WHERE id = $1`,
		id, status, nextAttemptAt, lastError,
	)
	return err
}

// PendingStats reports the number of pending events and the age of the oldest
// one relative to now, for backlog metrics. When no events are pending it
// returns (0, 0, nil). Age is derived from created_at so it reflects how long
// the oldest event has been waiting to be delivered.
func (r *Repository) PendingStats(ctx context.Context, now time.Time) (int, time.Duration, error) {
	var count int
	var oldestCreatedAt *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*), MIN(created_at)
		 FROM email_outbox
		 WHERE status = 'pending'`,
	).Scan(&count, &oldestCreatedAt)
	if err != nil {
		return 0, 0, err
	}
	if oldestCreatedAt == nil {
		return 0, 0, nil
	}
	age := now.Sub(*oldestCreatedAt)
	if age < 0 {
		age = 0
	}
	return count, age, nil
}
