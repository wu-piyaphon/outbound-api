CREATE TYPE signal_mode AS ENUM ('live', 'shadow');
ALTER TABLE signals ADD COLUMN mode signal_mode NOT NULL DEFAULT 'live';

CREATE TABLE shadow_exit_decisions (
  id UUID PRIMARY KEY,
  trade_id UUID NOT NULL REFERENCES trades(id) ON DELETE CASCADE,
  bar_time TIMESTAMPTZ NOT NULL,
  current_price DECIMAL(19,4) NOT NULL,
  peak_price DECIMAL(19,4) NOT NULL,
  current_stop DECIMAL(19,4),
  action VARCHAR(20) NOT NULL,
  reasoning TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_shadow_exit_decisions_trade ON shadow_exit_decisions(trade_id, bar_time);
