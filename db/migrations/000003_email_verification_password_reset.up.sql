-- Add email verification timestamp to users
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;

-- Email verification tokens: single-use hashed tokens for email confirmation
CREATE TABLE email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Password reset tokens: single-use hashed tokens for password recovery
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial indexes for efficient active-token lookup
CREATE INDEX idx_evt_user_active ON email_verification_tokens(user_id) WHERE used_at IS NULL;
CREATE INDEX idx_prt_user_active ON password_reset_tokens(user_id) WHERE used_at IS NULL;
