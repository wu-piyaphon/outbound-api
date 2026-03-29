CREATE TABLE watchlists (
    symbol VARCHAR(10) PRIMARY KEY,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);