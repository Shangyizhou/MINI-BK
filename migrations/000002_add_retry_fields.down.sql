DROP INDEX IF EXISTS idx_tasks_idempotency;
ALTER TABLE tasks DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE tasks DROP COLUMN IF EXISTS retry_interval_sec;
ALTER TABLE tasks DROP COLUMN IF EXISTS retry_count;
ALTER TABLE tasks DROP COLUMN IF EXISTS max_retries;
