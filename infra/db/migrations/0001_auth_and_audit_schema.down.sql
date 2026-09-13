-- Rollback for migration 0001: drop auth and audit schemas.
--
-- CASCADE drops all tables, indexes and sequences that belong to each schema.
-- This destroys all data in those schemas — intended only for development
-- and test environments.
--
-- This migration is applied inside a single transaction by cmd/migrate.
-- Do not add BEGIN/COMMIT here; the runner owns the transaction boundary.

-- Revoke the append-only grant before dropping the table (harmless if the
-- role does not exist, e.g. in a test database that uses a superuser).
-- Use DO block so a missing role does not abort the transaction.
DO $$
BEGIN
    REVOKE INSERT, SELECT ON audit.records FROM grindstats;
EXCEPTION WHEN undefined_object OR undefined_table THEN
    NULL; -- role or table does not exist; nothing to revoke
END;
$$;

DROP SCHEMA IF EXISTS audit CASCADE;
DROP SCHEMA IF EXISTS auth CASCADE;
