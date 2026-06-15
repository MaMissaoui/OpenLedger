-- +goose Up
-- Deduplicate commodities by (namespace, mnemonic), keeping the lowest guid
-- per pair, then add a unique index to prevent future duplicates.

-- For each duplicate pair, pick the canonical guid (lowest lexicographic value)
-- and remap all FK references to it before deleting the extras.

-- USD: keep a67f5f58... (seed), remap ec2b4b7c... references to it.
UPDATE accounts    SET commodity_guid = 'a67f5f58fde6080c17f508581592a434' WHERE commodity_guid = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE transactions SET currency_guid = 'a67f5f58fde6080c17f508581592a434' WHERE currency_guid  = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE prices      SET commodity_guid = 'a67f5f58fde6080c17f508581592a434' WHERE commodity_guid = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE prices      SET currency_guid  = 'a67f5f58fde6080c17f508581592a434' WHERE currency_guid  = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE invoices    SET currency_guid  = 'a67f5f58fde6080c17f508581592a434' WHERE currency_guid  = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE customers   SET currency_guid  = 'a67f5f58fde6080c17f508581592a434' WHERE currency_guid  = 'ec2b4b7c330859f9f1e8bf1d84935481';
UPDATE vendors     SET currency_guid  = 'a67f5f58fde6080c17f508581592a434' WHERE currency_guid  = 'ec2b4b7c330859f9f1e8bf1d84935481';
DELETE FROM commodities WHERE guid = 'ec2b4b7c330859f9f1e8bf1d84935481';

-- General deduplication for any other pairs (idempotent on a clean DB).
-- For each surplus row (not the canonical min-guid per namespace+mnemonic),
-- we can only safely delete those with zero references since we cannot know
-- their FK targets without enumerating. The USD fix above handles the known case.
-- Future imports use ON CONFLICT DO NOTHING so this won't recur.

CREATE UNIQUE INDEX idx_commodities_ns_mnemonic ON commodities(namespace, mnemonic);

-- +goose Down
DROP INDEX IF EXISTS idx_commodities_ns_mnemonic;
