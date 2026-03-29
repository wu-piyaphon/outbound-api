CREATE TABLE account_transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    type transfer_type NOT NULL,
    amount_thb DECIMAL(19, 4) NOT NULL,
    amount_usd DECIMAL(19, 4) NOT NULL,
    fee_thb DECIMAL(19, 4) NOT NULL,
    fee_usd DECIMAL(19, 4),
    exchange_rate DECIMAL(19, 4) NOT NULL,
    target_trades INTEGER NOT NULL,
    remaining_trades INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);