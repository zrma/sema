CREATE TABLE IF NOT EXISTS sema_repository_metadata (
    key text PRIMARY KEY,
    value text NOT NULL
)
-- sema:statement
INSERT INTO sema_repository_metadata (key, value)
VALUES ('schema_version', '1')
ON CONFLICT (key) DO NOTHING
-- sema:statement
CREATE TABLE IF NOT EXISTS sema_repository_scopes (
    scope text PRIMARY KEY CHECK (scope <> ''),
    version bigint NOT NULL DEFAULT 0 CHECK (version >= 0)
)
-- sema:statement
CREATE TABLE IF NOT EXISTS sema_repository_operations (
    scope text NOT NULL REFERENCES sema_repository_scopes (scope),
    operation_id text NOT NULL CHECK (operation_id <> ''),
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    operation_kind text NOT NULL CHECK (operation_kind <> ''),
    occurred_at timestamptz NOT NULL,
    version bigint,
    PRIMARY KEY (scope, operation_id),
    UNIQUE (scope, version),
    CHECK (version IS NULL OR version > 0)
)
-- sema:statement
CREATE TABLE IF NOT EXISTS sema_repository_resources (
    scope text NOT NULL REFERENCES sema_repository_scopes (scope),
    resource_kind text NOT NULL CHECK (resource_kind <> ''),
    resource_id text NOT NULL CHECK (resource_id <> ''),
    version bigint NOT NULL CHECK (version > 0),
    payload bytea NOT NULL,
    deleted boolean NOT NULL,
    PRIMARY KEY (scope, resource_kind, resource_id),
    CHECK (
        (deleted AND octet_length(payload) = 0) OR
        (NOT deleted AND octet_length(payload) > 0)
    )
)
-- sema:statement
CREATE INDEX IF NOT EXISTS sema_repository_resources_scope_order
ON sema_repository_resources (scope, resource_kind, resource_id)
-- sema:statement
CREATE TABLE IF NOT EXISTS sema_repository_audit (
    scope text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    operation_kind text NOT NULL CHECK (operation_kind <> ''),
    occurred_at timestamptz NOT NULL,
    resource_counts jsonb NOT NULL CHECK (jsonb_typeof(resource_counts) = 'object'),
    PRIMARY KEY (scope, version),
    FOREIGN KEY (scope, version)
        REFERENCES sema_repository_operations (scope, version)
)
