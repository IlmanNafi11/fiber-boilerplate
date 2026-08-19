// Package email holds domain types for durable email delivery via the outbox.
package email

import (
	"encoding/json"
	"time"
)

// Outbox event types. These form the contract between the auth service (which
// enqueues) and the dispatcher (which maps each type to an EmailSender call).
const (
	EventTypeVerification    = "verification"
	EventTypePasswordReset   = "password_reset"
	EventTypePasswordChanged = "password_changed"
)

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

// TokenPayload is the JSON payload for events that carry a single-use token
// (verification and password reset). It never appears in logs, and the payload
// is cleared on the row once the event is marked sent, so the token does not
// persist in plaintext after delivery.
type TokenPayload struct {
	Token string `json:"token"`
}

// NewTokenEvent builds a pending outbox event whose payload carries a token.
func NewTokenEvent(eventType, recipient, token string) (*OutboxEvent, error) {
	payload, err := json.Marshal(TokenPayload{Token: token})
	if err != nil {
		return nil, err
	}
	return &OutboxEvent{
		EventType: eventType,
		Recipient: recipient,
		Payload:   payload,
	}, nil
}

// NewNotificationEvent builds a pending outbox event with an empty payload,
// used for notifications that carry no token (e.g. password changed).
func NewNotificationEvent(eventType, recipient string) *OutboxEvent {
	return &OutboxEvent{
		EventType: eventType,
		Recipient: recipient,
		Payload:   []byte("{}"),
	}
}
