-- Email outbox: transactional outbox for durable auth email delivery.
-- Events are enqueued in the same DB transaction as the auth state change,
-- then claimed and dispatched by a worker with a lease.
CREATE TABLE email_outbox (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(64) NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    locked_by VARCHAR(128),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT email_outbox_status_check CHECK (status IN ('pending', 'sent', 'dead'))
);

-- Claim query filters pending rows due for delivery ordered by next_attempt_at.
-- Expired leases are reclaimed by comparing locked_at against a cutoff, so the
-- index also covers rows that are locked but stale.
CREATE INDEX idx_email_outbox_claim
    ON email_outbox (next_attempt_at)
    WHERE status = 'pending';
