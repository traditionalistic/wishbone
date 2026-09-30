package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/preview"
)

const realContract = "../../../contract"

// seeded is a server on the real contract with every contract screen seeded, as at startup.
func seeded(t *testing.T, extraOps ...map[string]ops.Handler) *env {
	t.Helper()
	newEnv(t) // empty tables
	e := newEnvWith(t, realContract, extraOps...)
	if err := e.srv.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) focus(src []byte) preview.Screen {
	e.t.Helper()
	sc, diags := e.srv.previewScreen(context.Background(), src)
	if len(diags) > 0 {
		e.t.Fatal(diags)
	}
	return sc
}

func contractScreen(t *testing.T, id string) []byte {
	t.Helper()
	b, err := os.ReadFile(realContract + "/screens/" + id + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ids(b preview.Board) []string {
	var out []string
	for _, sc := range b.Screens {
		out = append(out, sc.ID)
	}
	return out
}

// Links count in both directions: raffle.tickets links nowhere, yet raffle.main links to it, and slot.feed.top to
// raffle.main.
func TestBoardComponent(t *testing.T) {
	e := seeded(t)
	b := e.srv.board(context.Background(), e.focus(contractScreen(t, "raffle.tickets")))
	want := []string{"raffle.tickets", "raffle.main", "raffle.invites", "raffle.rules", "slot.feed.top"}
	if b.Focus != "raffle.tickets" || !slices.Equal(ids(b), want) {
		t.Fatalf("board %s %v, want %v", b.Focus, ids(b), want)
	}
	for _, sc := range b.Screens {
		if sc.Error != "" || sc.Document == nil || sc.Data == nil || sc.Title == "" {
			t.Fatalf("%s is not ready to render: %+v", sc.ID, sc)
		}
	}
	if !strings.Contains(string(b.Screens[1].Document), `"screen":"raffle.main"`) || len(b.Screens[1].Links) != 3 {
		t.Fatalf("a linked screen is its compiled draft: %.200s %v", b.Screens[1].Document, b.Screens[1].Links)
	}
}

func TestBoardGhostsAndCap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	source := func(id string, targets ...string) []byte {
		var script strings.Builder
		for _, to := range targets {
			fmt.Fprintf(&script, "navigate(%q)\n", to)
		}
		b, _ := json.Marshal(map[string]any{"id": id, "title": id, "script": "handler go() {\n" + script.String() + "}", "timelines": []any{},
			"root": map[string]any{"type": "stack", "id": "root", "on": map[string]string{"tap": "go()"}}})
		return b
	}
	broken := json.RawMessage(`{"id": "t.broken", "title": "Broken", "script": "state n = ", "timelines": [], "root": {"type": "stack", "id": "root"}}`)
	if _, err := e.srv.Store.CreateScreen(ctx, "t.broken", "Broken", broken, "test"); err != nil {
		t.Fatal(err)
	}
	b := e.srv.board(ctx, e.focus(source("t.lost", "t.nowhere", "t.broken")))
	if !slices.Equal(ids(b), []string{"t.lost", "t.broken", "t.nowhere"}) {
		t.Fatalf("board %v", ids(b))
	}
	if g := b.Screens[2]; g.Error != "screen not found" || g.Document != nil || g.Links == nil {
		t.Fatalf("a missing target is a ghost: %+v", g)
	}
	if d := b.Screens[1]; !strings.HasPrefix(d.Error, "draft does not compile: ") || len(d.Error) == len("draft does not compile: ") ||
		d.Title != "Broken" || d.Document != nil || d.Links == nil {
		t.Fatalf("a draft that does not compile: %+v", d)
	}

	var many []string
	for i := range 20 {
		id := fmt.Sprintf("t.c%02d", i)
		many = append(many, id)
		if _, err := e.srv.Store.CreateScreen(ctx, id, id, source(id), "test"); err != nil {
			t.Fatal(err)
		}
	}
	focus := e.focus(source("t.hub", many...))
	if b := e.srv.board(ctx, focus); !slices.Equal(ids(b), append([]string{"t.hub"}, many[:maxBoard-1]...)) {
		t.Fatalf("capped board %v", ids(b))
	}

	e.srv.Store.Close()
	if b := e.srv.board(ctx, focus); !slices.Equal(ids(b), []string{"t.hub"}) {
		t.Fatalf("a store failure leaves the focus alone: %v", ids(b))
	}
}

// Linked drafts resolve their data once per saved version.
func TestBoardCachesDrafts(t *testing.T) {
	var calls atomic.Int32 // raffle.main and raffle.rules read raffle.overview
	e := seeded(t, map[string]ops.Handler{"raffle.overview": func(context.Context, ops.Req) (ops.Result, error) {
		calls.Add(1)
		return ops.Fail("UPSTREAM_FAILED", "offline"), nil
	}})
	ctx := context.Background()
	focus := e.focus(contractScreen(t, "raffle.tickets"))
	e.srv.board(ctx, focus)
	e.srv.board(ctx, focus)
	if n := calls.Load(); n != 2 {
		t.Fatalf("two boards resolved raffle.overview %d times, want 2 (raffle.main and raffle.rules, once)", n)
	}
	if _, err := e.srv.Store.SaveDraft(ctx, "raffle.rules", nil, contractScreen(t, "raffle.rules"), "test"); err != nil {
		t.Fatal(err)
	}
	b := e.srv.board(ctx, focus)
	if n := calls.Load(); n != 3 {
		t.Fatalf("a saved draft resolves again, alone: %d calls, want 3", n)
	}
	if !slices.Equal(ids(b), []string{"raffle.tickets", "raffle.main", "raffle.invites", "raffle.rules", "slot.feed.top"}) {
		t.Fatalf("board %v", ids(b))
	}
}
