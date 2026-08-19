// Package email holds domain types for durable email delivery via the outbox.
package email

import "time"

// Outbox status values.
const (
	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusDead    = "dead"
)

// OutboxEvent is a single durable email delivery record. Events are enqueued
// in the same transaction as the auth state change that produced them and
// dispatched later by a leased worker (at-least-once delivery).
type OutboxEvent struct {
	ID            string
	EventType     string
	Recipient     string
	Payload       []byte // JSON-encoded, event-type-specific content
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	LockedAt      *time.Time
	LockedBy      *string
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
