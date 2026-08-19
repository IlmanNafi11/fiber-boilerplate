package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	emaildomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	"go.uber.org/zap"
)

// OutboxRepo is the subset of the outbox repository the dispatcher needs to
// lease due events and record delivery outcomes.
type OutboxRepo interface {
	ClaimBatch(ctx context.Context, workerID string, limit int, lease time.Duration, now time.Time) ([]*emaildomain.OutboxEvent, error)
	MarkSent(ctx context.Context, id string) error
	MarkRetry(ctx context.Context, id string, nextAttemptAt time.Time, lastError string, dead bool) error
}

// DispatcherConfig tunes the dispatch loop and retry policy.
type DispatcherConfig struct {
	Interval    time.Duration // how often to poll for due events
	BatchSize   int           // max events leased per poll
	Lease       time.Duration // lease duration held while sending
	SendTimeout time.Duration // per-event send deadline
	MaxAttempts int           // attempts before dead-lettering
	BaseBackoff time.Duration // first retry delay; doubled per attempt
	MaxBackoff  time.Duration // retry delay ceiling
}

// errPermanent marks a failure that must not be retried (e.g. unknown event
// type). It short-circuits to dead-letter regardless of remaining attempts.
var errPermanent = errors.New("permanent delivery failure")

// Dispatcher drains the email outbox: it leases due events, sends each via the
// EmailSender, and records sent/retry/dead outcomes. Delivery is at-least-once.
type Dispatcher struct {
	repo     OutboxRepo
	sender   EmailSender
	cfg      DispatcherConfig
	logger   *zap.Logger
	workerID string
	now      func() time.Time
}

// NewDispatcher builds a dispatcher with a unique worker ID used for leasing.
func NewDispatcher(repo OutboxRepo, sender EmailSender, cfg DispatcherConfig, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		repo:     repo,
		sender:   sender,
		cfg:      cfg,
		logger:   logger,
		workerID: uuid.NewString(),
		now:      time.Now,
	}
}

// NewFromConfig builds a dispatcher, deriving its retry/lease policy from the
// application's EmailConfig. Used by server wiring.
func NewFromConfig(repo OutboxRepo, sender EmailSender, cfg config.EmailConfig, logger *zap.Logger) *Dispatcher {
	return NewDispatcher(repo, sender, DispatcherConfig{
		Interval:    cfg.OutboxDispatchInterval,
		BatchSize:   cfg.OutboxBatchSize,
		Lease:       cfg.OutboxLease,
		SendTimeout: cfg.OutboxSendTimeout,
		MaxAttempts: cfg.OutboxMaxAttempts,
		BaseBackoff: cfg.OutboxBaseBackoff,
		MaxBackoff:  cfg.OutboxMaxBackoff,
	}, logger)
}

// Run polls the outbox until ctx is cancelled. On cancellation it stops
// claiming new work and returns; in-flight leases expire and are reclaimed on
// restart, so no event is lost.
func (d *Dispatcher) Run(ctx context.Context) {
	d.logger.Info("email dispatcher started",
		zap.String("worker_id", d.workerID),
		zap.Duration("interval", d.cfg.Interval),
	)

	ticker := time.NewTicker(d.cfg.Interval)
	defer ticker.Stop()

	for {
		if _, err := d.dispatchOnce(ctx); err != nil && ctx.Err() == nil {
			d.logger.Error("outbox dispatch pass failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			d.logger.Info("email dispatcher stopped", zap.String("worker_id", d.workerID))
			return
		case <-ticker.C:
		}
	}
}

// dispatchOnce leases one batch of due events and delivers each, returning the
// number of events processed. It is the unit of work exercised by tests.
func (d *Dispatcher) dispatchOnce(ctx context.Context) (int, error) {
	events, err := d.repo.ClaimBatch(ctx, d.workerID, d.cfg.BatchSize, d.cfg.Lease, d.now())
	if err != nil {
		return 0, fmt.Errorf("claim batch: %w", err)
	}

	for _, e := range events {
		d.deliver(ctx, e)
	}
	return len(events), nil
}

// deliver sends a single event and records its outcome. Success acks the event;
// failure either reschedules with backoff or dead-letters when attempts are
// exhausted or the failure is permanent.
func (d *Dispatcher) deliver(ctx context.Context, e *emaildomain.OutboxEvent) {
	sendCtx, cancel := context.WithTimeout(ctx, d.cfg.SendTimeout)
	defer cancel()

	if err := d.send(sendCtx, e); err != nil {
		attempts := e.Attempts + 1
		dead := errors.Is(err, errPermanent) || attempts >= d.cfg.MaxAttempts
		nextAttemptAt := d.now().Add(d.backoff(e.Attempts))

		if markErr := d.repo.MarkRetry(ctx, e.ID, nextAttemptAt, sanitizeError(err), dead); markErr != nil {
			d.logger.Error("mark retry failed", zap.String("event_id", e.ID), zap.Error(markErr))
			return
		}
		d.logger.Warn("outbox event delivery failed",
			zap.String("event_id", e.ID),
			zap.String("event_type", e.EventType),
			zap.Int("attempts", attempts),
			zap.Bool("dead", dead),
			zap.String("error", sanitizeError(err)),
		)
		return
	}

	if err := d.repo.MarkSent(ctx, e.ID); err != nil {
		d.logger.Error("mark sent failed", zap.String("event_id", e.ID), zap.Error(err))
		return
	}
	d.logger.Info("outbox event delivered",
		zap.String("event_id", e.ID),
		zap.String("event_type", e.EventType),
	)
}

// send maps an allowlisted event type to the matching EmailSender call. An
// unknown type is a permanent failure.
func (d *Dispatcher) send(ctx context.Context, e *emaildomain.OutboxEvent) error {
	switch e.EventType {
	case emaildomain.EventTypeVerification:
		token, err := tokenFromPayload(e.Payload)
		if err != nil {
			return err
		}
		return d.sender.SendVerificationEmail(ctx, e.Recipient, token)
	case emaildomain.EventTypePasswordReset:
		token, err := tokenFromPayload(e.Payload)
		if err != nil {
			return err
		}
		return d.sender.SendPasswordResetEmail(ctx, e.Recipient, token)
	case emaildomain.EventTypePasswordChanged:
		return d.sender.SendPasswordChangedNotification(ctx, e.Recipient)
	default:
		return fmt.Errorf("%w: unknown event type %q", errPermanent, e.EventType)
	}
}

// backoff returns the delay before the next attempt: BaseBackoff doubled per
// prior attempt, capped at MaxBackoff.
func (d *Dispatcher) backoff(priorAttempts int) time.Duration {
	delay := d.cfg.BaseBackoff
	for range priorAttempts {
		delay *= 2
		if delay >= d.cfg.MaxBackoff {
			return d.cfg.MaxBackoff
		}
	}
	if delay > d.cfg.MaxBackoff {
		return d.cfg.MaxBackoff
	}
	return delay
}

func tokenFromPayload(payload []byte) (string, error) {
	var p emaildomain.TokenPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		// Malformed payload can never succeed; treat as permanent.
		return "", fmt.Errorf("%w: decode payload: %v", errPermanent, err)
	}
	return p.Token, nil
}

// sanitizeError returns the error's message for diagnostics. Callers must never
// place secrets (tokens, payloads) in error strings; the outbox stores this in
// last_error, which is surfaced in logs and metrics.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
