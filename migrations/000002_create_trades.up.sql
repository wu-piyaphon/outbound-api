CREATE TABLE trades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    parent_id UUID REFERENCES trades (id) ON DELETE RESTRICT,
    signal_id UUID REFERENCES signals (id) ON DELETE RESTRICT,
    alpaca_order_id VARCHAR(50) UNIQUE,
    symbol VARCHAR(10) NOT NULL,
    side side NOT NULL,
    quantity DECIMAL(19, 4) NOT NULL,
    price_per_unit DECIMAL(19, 4),
    avg_fill_price DECIMAL(19, 4),
    commission_fee DECIMAL(19, 4),
    fx_fee_amortized DECIMAL(19, 4),
    status order_status NOT NULL,
    metadata JSONB,
    filled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);