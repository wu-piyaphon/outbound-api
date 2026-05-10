DROP INDEX IF EXISTS idx_shadow_exit_decisions_trade;
DROP TABLE IF EXISTS shadow_exit_decisions;
ALTER TABLE signals DROP COLUMN IF EXISTS mode;
DROP TYPE IF EXISTS signal_mode;
