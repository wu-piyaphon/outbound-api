ALTER TABLE trades
ADD COLUMN account_transfer_id UUID NOT NULL REFERENCES account_transfers (id) ON DELETE RESTRICT;