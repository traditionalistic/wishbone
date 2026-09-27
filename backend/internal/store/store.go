// Package store is the Postgres persistence: screens with drafts, immutable revisions, call idempotency and audit.
package store

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound   = errors.New("screen not found")
	ErrExists     = errors.New("screen already exists")
	ErrNoRevision = errors.New("revision not found")
	ErrInFlight   = errors.New("call in flight")
	ErrMismatch   = errors.New("idempotency key reused with other arguments")
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ db *pgxpool.Pool }

type Screen struct {
	ID, Title    string
	Draft        json.RawMessage // nil in Screens()
	UpdatedAt    time.Time
	PublishedRev *int
}

type Revision struct {
	Screen      string
	Rev         int
	Source      json.RawMessage
	Document    json.RawMessage
	ETag        string
	RuntimeMin  string
	Meta        json.RawMessage // opaque to the store (the API's ops/data/params)
	PublishedAt time.Time
}

// Build compiles a source into revision rev; it fills Document, ETag, RuntimeMin and Meta.
type Build func(source json.RawMessage, rev int) (*Revision, error)

// Open connects and applies pending migrations.
func Open(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			var done bool
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7461)`); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, e.Name()).Scan(&done); err != nil || done {
				return err
			}
			sql, err := migrations.ReadFile("migrations/" + e.Name())
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, e.Name())
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
	}
	return nil
}

// EnsureDatabase creates the database named in url when it does not exist (via the `postgres` database).
func EnsureDatabase(ctx context.Context, url string) error {
	return withServer(ctx, url, func(c *pgx.Conn, name string) error {
		var exists bool
		if err := c.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil || exists {
			return err
		}
		_, err := c.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
		return err
	})
}

// DropDatabase drops the database named in url (tests).
func DropDatabase(ctx context.Context, url string) error {
	return withServer(ctx, url, func(c *pgx.Conn, name string) error {
		_, err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		return err
	})
}

func withServer(ctx context.Context, url string, f func(c *pgx.Conn, name string) error) error {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return err
	}
	name := cfg.Database
	cfg.Database = "postgres"
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	return f(c, name)
}

func audit(ctx context.Context, tx pgx.Tx, actor, action, entity string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit (actor, action, entity, details) VALUES ($1, $2, $3, $4)`, actor, action, entity, details)
	return err
}

const publishedRev = `(SELECT max(rev) FROM revisions r WHERE r.screen_id = s.id)`

func (s *Store) Screens(ctx context.Context) ([]Screen, error) {
	rows, err := s.db.Query(ctx, `SELECT id, title, updated_at, `+publishedRev+` FROM screens s ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Screen, error) {
		var sc Screen
		return sc, r.Scan(&sc.ID, &sc.Title, &sc.UpdatedAt, &sc.PublishedRev)
	})
}

func (s *Store) Screen(ctx context.Context, id string) (*Screen, error) {
	sc := &Screen{ID: id}
	err := s.db.QueryRow(ctx, `SELECT title, draft, updated_at, `+publishedRev+` FROM screens s WHERE id = $1`, id).
		Scan(&sc.Title, &sc.Draft, &sc.UpdatedAt, &sc.PublishedRev)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sc, err
}

func (s *Store) CreateScreen(ctx context.Context, id, title string, draft json.RawMessage, actor string) (*Screen, error) {
	sc := &Screen{ID: id, Title: title, Draft: draft}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO screens (id, title, draft) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING RETURNING updated_at`,
			id, title, draft).Scan(&sc.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, actor, "create", id, nil)
	})
	if err != nil {
		return nil, err
	}
	return sc, nil
}

// SaveDraft replaces the draft (and the title when non-nil).
func (s *Store) SaveDraft(ctx context.Context, id string, title *string, draft json.RawMessage, actor string) (time.Time, error) {
	var at time.Time
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE screens SET draft = $2, title = coalesce($3, title), updated_at = clock_timestamp() WHERE id = $1 RETURNING updated_at`,
			id, draft, title).Scan(&at)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return audit(ctx, tx, actor, "save", id, nil)
	})
	return at, err
}

const revCols = `screen_id, rev, source, document, etag, runtime_min, meta, published_at`

func scanRev(row pgx.Row) (*Revision, error) {
	r := &Revision{}
	err := row.Scan(&r.Screen, &r.Rev, &r.Source, &r.Document, &r.ETag, &r.RuntimeMin, &r.Meta, &r.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoRevision
	}
	return r, err
}

// Live is the newest revision; ErrNotFound when the screen has none.
func (s *Store) Live(ctx context.Context, id string) (*Revision, error) {
	r, err := scanRev(s.db.QueryRow(ctx, `SELECT `+revCols+` FROM revisions WHERE screen_id = $1 ORDER BY rev DESC LIMIT 1`, id))
	if errors.Is(err, ErrNoRevision) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) Revision(ctx context.Context, id string, rev int) (*Revision, error) {
	return scanRev(s.db.QueryRow(ctx, `SELECT `+revCols+` FROM revisions WHERE screen_id = $1 AND rev = $2`, id, rev))
}

// Revisions lists revisions newest first, without sources and documents.
func (s *Store) Revisions(ctx context.Context, id string) ([]Revision, error) {
	rows, err := s.db.Query(ctx, `SELECT rev, etag, runtime_min, published_at FROM revisions WHERE screen_id = $1 ORDER BY rev DESC`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Revision, error) {
		r := Revision{Screen: id}
		return r, row.Scan(&r.Rev, &r.ETag, &r.RuntimeMin, &r.PublishedAt)
	})
}

// Publish builds the next revision in one transaction: from = 0 publishes the draft, from = N the source of
// revision N; resetDraft also makes that source the draft (rollback). Build errors pass through unchanged.
func (s *Store) Publish(ctx context.Context, id string, from int, resetDraft bool, actor string, build Build) (*Revision, error) {
	var out *Revision
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var src json.RawMessage
		err := tx.QueryRow(ctx, `SELECT draft FROM screens WHERE id = $1 FOR UPDATE`, id).Scan(&src)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if from > 0 {
			err := tx.QueryRow(ctx, `SELECT source FROM revisions WHERE screen_id = $1 AND rev = $2`, id, from).Scan(&src)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoRevision
			}
			if err != nil {
				return err
			}
		}
		var next int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(rev), 0) + 1 FROM revisions WHERE screen_id = $1`, id).Scan(&next); err != nil {
			return err
		}
		r, err := build(src, next)
		if err != nil {
			return err
		}
		r.Screen, r.Rev, r.Source = id, next, src
		if err := tx.QueryRow(ctx, `INSERT INTO revisions (screen_id, rev, source, document, etag, runtime_min, meta)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING published_at`,
			id, next, src, r.Document, r.ETag, r.RuntimeMin, r.Meta).Scan(&r.PublishedAt); err != nil {
			return err
		}
		action := "publish"
		if resetDraft {
			action = "rollback"
			if _, err := tx.Exec(ctx, `UPDATE screens SET draft = $2, updated_at = clock_timestamp() WHERE id = $1`, id, src); err != nil {
				return err
			}
		}
		out = r
		return audit(ctx, tx, actor, action, id, map[string]any{"rev": next, "from": from})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Reserve claims (key, op) for a write call. It returns the stored response when the call already completed,
// nil when the caller now owns the reservation, ErrInFlight or ErrMismatch otherwise.
func (s *Store) Reserve(ctx context.Context, key, op string, hash []byte) (json.RawMessage, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO idempotency (key, op, request_hash, status) VALUES ($1, $2, $3, 'in_flight') ON CONFLICT DO NOTHING`, key, op, hash)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	var (
		stored []byte
		status string
		resp   json.RawMessage
	)
	err = s.db.QueryRow(ctx, `SELECT request_hash, status, response FROM idempotency WHERE key = $1 AND op = $2`, key, op).Scan(&stored, &status, &resp)
	switch {
	case errors.Is(err, pgx.ErrNoRows): // released between the two statements
		return nil, ErrInFlight
	case err != nil:
		return nil, err
	case !bytes.Equal(stored, hash):
		return nil, ErrMismatch
	case status != "done":
		return nil, ErrInFlight
	}
	return resp, nil
}

func (s *Store) Complete(ctx context.Context, key, op string, response json.RawMessage) error {
	_, err := s.db.Exec(ctx, `UPDATE idempotency SET status = 'done', response = $3 WHERE key = $1 AND op = $2`, key, op, response)
	return err
}

func (s *Store) Release(ctx context.Context, key, op string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM idempotency WHERE key = $1 AND op = $2 AND status = 'in_flight'`, key, op)
	return err
}

// ClearInFlight drops reservations orphaned by a previous process.
// ponytail: single-instance server; with several instances expire in_flight rows by age instead.
func (s *Store) ClearInFlight(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM idempotency WHERE status = 'in_flight'`)
	return err
}
