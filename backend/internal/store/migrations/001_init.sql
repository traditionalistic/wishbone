-- json (not jsonb) for sources, documents and stored call responses: byte-exact, so ETags hash exactly what is
-- served, authors' key order survives and idempotent replays repeat the original bytes.
CREATE TABLE screens (
    id         text PRIMARY KEY,
    title      text NOT NULL,
    draft      json NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE revisions (
    screen_id    text NOT NULL REFERENCES screens (id),
    rev          int NOT NULL,
    source       json NOT NULL,
    document     json NOT NULL,
    etag         text NOT NULL,
    runtime_min  text NOT NULL,
    meta         jsonb NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (screen_id, rev)
);

CREATE TABLE idempotency (
    key          text NOT NULL,
    op           text NOT NULL,
    request_hash bytea NOT NULL,
    status       text NOT NULL CHECK (status IN ('in_flight', 'done')),
    response     json,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key, op)
);

CREATE TABLE audit (
    id      bigserial PRIMARY KEY,
    actor   text NOT NULL,
    action  text NOT NULL,
    entity  text NOT NULL,
    at      timestamptz NOT NULL DEFAULT now(),
    details jsonb NOT NULL DEFAULT '{}'
);
