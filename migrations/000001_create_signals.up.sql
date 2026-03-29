CREATE TABLE signals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    symbol VARCHAR(10) NOT NULL,
    side side NOT NULL,
    price_at_signal decimal(19, 4) NOT NULL,
    indicators JSONB NOT NULL,
    is_executed BOOLEAN DEFAULT FALSE,
    reasoning TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);