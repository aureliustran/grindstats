-- Migration 0001: auth and audit schemas for the auth epic (run 1).
--
-- Schemas created:
--   auth  — user accounts, OAuth identities, single-use link tokens
--   audit — append-only audit log (FR-45)
--
-- This migration is applied inside a single transaction by cmd/migrate.
-- Do not add BEGIN/COMMIT here; the runner owns the transaction boundary.

CREATE SCHEMA auth;
CREATE SCHEMA audit;

-- ---------------------------------------------------------------------------
-- auth.users
--
-- email_lower carries the UNIQUE constraint (not email) so that
-- "Taken@Example.com" and "taken@example.com" collide (FR-01).
--
-- password_hash is NULL for OAuth-only accounts: a NOT NULL '' would make
-- "has no password" indistinguishable from "has an empty password".
--
-- role and status are char(8) compact codes (audit-and-errors.md §6).
-- Both are NOT NULL with no database default — the application supplies the
-- compact code and a DB-level default would be a second place that decides
-- what role/status a new account gets (FR-04).
-- ---------------------------------------------------------------------------
CREATE TABLE auth.users (
    id                uuid        PRIMARY KEY,
    email             text        NOT NULL,
    email_lower       text        NOT NULL UNIQUE,
    password_hash     text        NULL,
    role              char(8)     NOT NULL,
    status            char(8)     NOT NULL,
    email_verified_at timestamptz NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- auth.oauth_identities
--
-- provider is a char(8) compact code for the LinkedProvider enum.
-- subject is the provider's own stable identifier for the user.
-- The UNIQUE (provider, subject) constraint prevents double-linking.
-- ---------------------------------------------------------------------------
CREATE TABLE auth.oauth_identities (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    provider   char(8)     NOT NULL,
    subject    text        NOT NULL,
    email      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, subject)
);

-- ---------------------------------------------------------------------------
-- auth.link_tokens
--
-- One table for all three single-use emailed links (SEC-04).
-- Only the SHA-256 hash of the token is stored; the raw token is emailed.
-- kind is a char(8) compact code for the LinkKind enum.
-- payload holds OAuth-link metadata: {provider, subject, email}.
-- ---------------------------------------------------------------------------
CREATE TABLE auth.link_tokens (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    kind        char(8)     NOT NULL,
    token_hash  bytea       NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz NULL,
    payload     jsonb       NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON auth.link_tokens (user_id, kind);

-- ---------------------------------------------------------------------------
-- audit.records
--
-- Append-only (FR-45, SEC-07).  The application's DB role is granted
-- INSERT and SELECT only; no API path updates or deletes a row here.
--
-- All char(8) columns store compact codes from the generated audit model.
-- message is always rendered in en-US (backend.md §5a / CLAUDE.md).
-- ---------------------------------------------------------------------------
CREATE TABLE audit.records (
    id             bigserial   PRIMARY KEY,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    event_code     char(8)     NOT NULL,
    actor_type     char(8)     NOT NULL,
    outcome        char(8)     NOT NULL,
    severity       char(8)     NOT NULL,
    actor_user_id  uuid        NULL,
    target_user_id uuid        NULL,
    request_id     text        NULL,
    fields         jsonb       NOT NULL DEFAULT '{}',
    message        text        NOT NULL
);

CREATE INDEX ON audit.records (occurred_at DESC);
CREATE INDEX ON audit.records (target_user_id, occurred_at DESC);
CREATE INDEX ON audit.records (event_code, occurred_at DESC);

-- Append-only grant (FR-45).
-- In a superuser compose setup this statement is inert locally, but it
-- documents the intended production privilege boundary: no UPDATE or DELETE
-- on audit.records for the application role.  A real deployment role inherits
-- these grants and must not be given UPDATE/DELETE on this table.
GRANT INSERT, SELECT ON audit.records TO grindstats;
