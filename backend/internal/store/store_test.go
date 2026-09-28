package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

var testURL = "postgres:///wishbone_store_test?sslmode=disable"

func TestMain(m *testing.M) {
	if u := os.Getenv("WISHBONE_TEST_DATABASE_URL"); u != "" {
		testURL = u
	}
	ctx := context.Background()
	if err := DropDatabase(ctx, testURL); err != nil {
		if os.Getenv("WISHBONE_REQUIRE_DB") == "1" {
			fmt.Println("FAIL store tests: postgres unavailable and WISHBONE_REQUIRE_DB=1:", err)
			os.Exit(1)
		}
		fmt.Println("SKIP store tests: postgres unavailable:", err)
		os.Exit(0)
	}
	if err := EnsureDatabase(ctx, testURL); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = DropDatabase(ctx, testURL)
	os.Exit(code)
}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), testURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// fresh opens the store on empty tables (so -count=N reruns see no earlier state).
func fresh(t *testing.T) *Store {
	t.Helper()
	s := open(t)
	if _, err := s.db.Exec(ctx, `TRUNCATE screens, revisions, idempotency, audit`); err != nil {
		t.Fatal(err)
	}
	return s
}

var ctx = context.Background()

func TestMigrationsIdempotent(t *testing.T) {
	s := open(t)
	open(t)
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != len(migrationFiles(t)) {
		t.Fatalf("schema_migrations = %d (%v), want %d", n, err, len(migrationFiles(t)))
	}
}

func migrationFiles(t *testing.T) []string {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestScreens(t *testing.T) {
	s := fresh(t)
	d1 := json.RawMessage(`{"id":"s.one","root":{"b":1,"a":2}}`)
	sc, err := s.CreateScreen(ctx, "s.one", "One", d1, "admin")
	if err != nil || sc.ID != "s.one" || sc.Title != "One" || sc.PublishedRev != nil {
		t.Fatalf("create: %+v %v", sc, err)
	}
	if _, err := s.CreateScreen(ctx, "s.one", "Again", d1, "admin"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate create: %v", err)
	}
	got, err := s.Screen(ctx, "s.one")
	if err != nil || string(got.Draft) != string(d1) || got.Title != "One" {
		t.Fatalf("get: %+v %v (the draft must keep its bytes)", got, err)
	}
	title := "Uno"
	at, err := s.SaveDraft(ctx, "s.one", &title, json.RawMessage(`{"v":2}`), "admin")
	if err != nil || !at.After(got.UpdatedAt) {
		t.Fatalf("save: %v %v (was %v)", at, err, got.UpdatedAt)
	}
	if _, err := s.SaveDraft(ctx, "s.none", nil, json.RawMessage(`{}`), "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("save unknown: %v", err)
	}
	if _, err := s.Screen(ctx, "s.none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get unknown: %v", err)
	}
	got, _ = s.Screen(ctx, "s.one")
	if got.Title != "Uno" || string(got.Draft) != `{"v":2}` {
		t.Fatalf("after save: %+v", got)
	}
	list, err := s.Screens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		found = found || (x.ID == "s.one" && x.Title == "Uno" && x.PublishedRev == nil)
	}
	if !found {
		t.Fatalf("list: %+v", list)
	}
}

// fake build: the document embeds the rev and the source
func build(src json.RawMessage, rev int) (*Revision, error) {
	if string(src) == `{"bad":true}` {
		return nil, errors.New("does not compile")
	}
	return &Revision{Document: json.RawMessage(fmt.Sprintf(`{"rev":%d,"src":%s}`, rev, src)), ETag: fmt.Sprintf("%d-x", rev),
		RuntimeMin: "1.0.0", Meta: json.RawMessage(`{"ops":["demo.claimOffer"]}`)}, nil
}

func TestPublishAndRollback(t *testing.T) {
	s := fresh(t)
	d1, d2 := json.RawMessage(`{"n":1}`), json.RawMessage(`{"n":2}`)
	if _, err := s.CreateScreen(ctx, "s.pub", "Pub", d1, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Live(ctx, "s.pub"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("live before publish: %v", err)
	}
	r1, err := s.Publish(ctx, "s.pub", 0, false, "admin", build)
	if err != nil || r1.Rev != 1 || r1.ETag != "1-x" || r1.PublishedAt.IsZero() {
		t.Fatalf("publish 1: %+v %v", r1, err)
	}
	if _, err := s.SaveDraft(ctx, "s.pub", nil, d2, "admin"); err != nil {
		t.Fatal(err)
	}
	if r2, err := s.Publish(ctx, "s.pub", 0, false, "admin", build); err != nil || r2.Rev != 2 {
		t.Fatalf("publish 2: %+v %v", r2, err)
	}
	live, err := s.Live(ctx, "s.pub")
	if err != nil || live.Rev != 2 || string(live.Document) != `{"rev":2,"src":{"n":2}}` || string(live.Meta) != `{"ops": ["demo.claimOffer"]}` {
		t.Fatalf("live: %+v %v", live, err)
	}
	revs, err := s.Revisions(ctx, "s.pub")
	if err != nil || len(revs) != 2 || revs[0].Rev != 2 || revs[1].Rev != 1 || revs[0].RuntimeMin != "1.0.0" {
		t.Fatalf("revisions: %+v %v", revs, err)
	}

	r3, err := s.Publish(ctx, "s.pub", 1, true, "admin", build)
	if err != nil || r3.Rev != 3 || string(r3.Source) != `{"n":1}` {
		t.Fatalf("rollback: %+v %v", r3, err)
	}
	sc, _ := s.Screen(ctx, "s.pub")
	if string(sc.Draft) != `{"n":1}` || sc.PublishedRev == nil || *sc.PublishedRev != 3 {
		t.Fatalf("after rollback the old source is the draft: %+v", sc)
	}
	if r4, err := s.Publish(ctx, "s.pub", 3, false, "system", build); err != nil || r4.Rev != 4 {
		t.Fatalf("republish without draft reset: %+v %v", r4, err)
	}

	if _, err := s.SaveDraft(ctx, "s.pub", nil, json.RawMessage(`{"bad":true}`), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, "s.pub", 0, false, "admin", build); err == nil || err.Error() != "does not compile" {
		t.Fatalf("build error must pass through: %v", err)
	}
	if live, _ := s.Live(ctx, "s.pub"); live.Rev != 4 {
		t.Fatalf("a failed build must not create a revision: %d", live.Rev)
	}
	if _, err := s.Publish(ctx, "s.nope", 0, false, "admin", build); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown screen: %v", err)
	}
	if _, err := s.Publish(ctx, "s.pub", 99, true, "admin", build); !errors.Is(err, ErrNoRevision) {
		t.Fatalf("unknown revision: %v", err)
	}
	if _, err := s.Revision(ctx, "s.pub", 99); !errors.Is(err, ErrNoRevision) {
		t.Fatalf("revision 99: %v", err)
	}
	var n int
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM audit WHERE entity = 's.pub'`).Scan(&n)
	if n != 7 { // create, 2 saves, 2 publishes, rollback, republish
		t.Fatalf("audit rows %d", n)
	}
}

func TestUnpublish(t *testing.T) {
	s := fresh(t)
	d1 := json.RawMessage(`{"n":1}`)
	if _, err := s.CreateScreen(ctx, "s.off", "Off", d1, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, "s.off", 0, false, "admin", build); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Live(ctx, "s.off"); err != nil {
		t.Fatalf("live before unpublish: %v", err)
	}
	if err := s.Unpublish(ctx, "s.off", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Live(ctx, "s.off"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("live after unpublish: %v", err)
	}
	sc, err := s.Screen(ctx, "s.off")
	if err != nil || !sc.Unpublished || sc.PublishedRev == nil {
		t.Fatalf("screen after unpublish: %+v %v", sc, err)
	}
	list, err := s.Screens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		found = found || (x.ID == "s.off" && x.Unpublished)
	}
	if !found {
		t.Fatalf("list after unpublish: %+v", list)
	}
	// unpublishing again is a no-op, not an error
	if err := s.Unpublish(ctx, "s.off", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, "s.off", 0, false, "admin", build); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Live(ctx, "s.off"); err != nil {
		t.Fatalf("publish clears unpublished_at: %v", err)
	}
	sc, _ = s.Screen(ctx, "s.off")
	if sc.Unpublished {
		t.Fatalf("publish must clear Unpublished: %+v", sc)
	}
	if err := s.Unpublish(ctx, "s.nope", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown screen: %v", err)
	}
}

func TestIdempotency(t *testing.T) {
	s := fresh(t)
	h1, h2 := []byte{1}, []byte{2}
	if done, err := s.Reserve(ctx, "k1", "demo.subscribe", h1); err != nil || done != nil {
		t.Fatalf("first reserve: %s %v", done, err)
	}
	if _, err := s.Reserve(ctx, "k1", "demo.subscribe", h1); !errors.Is(err, ErrInFlight) {
		t.Fatalf("second reserve: %v", err)
	}
	if _, err := s.Reserve(ctx, "k1", "demo.subscribe", h2); !errors.Is(err, ErrMismatch) {
		t.Fatalf("other args: %v", err)
	}
	if done, err := s.Reserve(ctx, "k1", "demo.claimOffer", h2); err != nil || done != nil {
		t.Fatalf("same key, other op: %s %v", done, err)
	}
	if err := s.Complete(ctx, "k1", "demo.subscribe", json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if done, err := s.Reserve(ctx, "k1", "demo.subscribe", h1); err != nil || string(done) != `{"ok":true}` {
		t.Fatalf("replay: %s %v", done, err)
	}
	if err := s.Release(ctx, "k1", "demo.claimOffer"); err != nil {
		t.Fatal(err)
	}
	if done, err := s.Reserve(ctx, "k1", "demo.claimOffer", h1); err != nil || done != nil {
		t.Fatalf("after release: %s %v", done, err)
	}
	if err := s.ClearInFlight(ctx); err != nil {
		t.Fatal(err)
	}
	if done, err := s.Reserve(ctx, "k1", "demo.claimOffer", h2); err != nil || done != nil {
		t.Fatalf("in-flight rows are cleared at startup: %s %v", done, err)
	}
	if done, err := s.Reserve(ctx, "k1", "demo.subscribe", h1); err != nil || done == nil {
		t.Fatalf("done rows survive ClearInFlight: %s %v", done, err)
	}
}

func TestPruneIdempotency(t *testing.T) {
	s := fresh(t)
	for _, k := range []string{"old", "new"} {
		if _, err := s.Reserve(ctx, k, "demo.subscribe", []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(ctx, `UPDATE idempotency SET created_at = now() - interval '25 hours' WHERE key = 'old'`); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneIdempotency(ctx, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var keys []string
	rows, _ := s.db.Query(ctx, `SELECT key FROM idempotency`)
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		keys = append(keys, k)
	}
	if rows.Err() != nil || len(keys) != 1 || keys[0] != "new" {
		t.Fatalf("after prune: %v %v", keys, rows.Err())
	}
}
