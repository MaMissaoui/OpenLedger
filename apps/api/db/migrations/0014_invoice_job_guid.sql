-- +goose Up
ALTER TABLE invoices ADD COLUMN job_guid CHAR(32) REFERENCES jobs(guid) ON DELETE SET NULL;
CREATE INDEX idx_invoices_job ON invoices(job_guid) WHERE job_guid IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_invoices_job;
ALTER TABLE invoices DROP COLUMN job_guid;
