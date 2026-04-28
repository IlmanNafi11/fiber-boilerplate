-- Products table: template CRUD resource demonstrating full-stack boilerplate pattern
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Partial index for active products list (D-21)
CREATE INDEX idx_products_active_created ON products (created_at DESC) WHERE deleted_at IS NULL;

-- Index for owner lookups
CREATE INDEX idx_products_user_id ON products(user_id);
