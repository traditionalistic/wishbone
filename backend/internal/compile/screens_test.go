package compile

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Every shipped screen compiles against the real contract (not only at server seed time), lazy-scroll list rule included.
func TestContractScreensCompile(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(contractDir, "screens", "*.json"))
	if len(files) == 0 {
		t.Fatal("no contract screens found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, diags := Screen(c, src, 1); len(diags) > 0 {
			t.Errorf("%s: %v", filepath.Base(f), diags)
		}
	}
}

// raffle.main's transitions (the storyboard's edges): literal navigate/present targets, by the node or handler that fires them.
func TestScreenLinks(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(contractDir, "screens", "raffle.main.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, m, diags := Screen(c, src, 1)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	want := []Link{{"raffle.invites", "navigate", "node:invites"}, {"raffle.rules", "present", "handler:rules"}, {"raffle.tickets", "navigate", "node:progress"}}
	if !slices.Equal(m.Links, want) {
		t.Fatalf("raffle.main links %+v, want %+v", m.Links, want)
	}

	// a repeated literal is one link, a link inside `if` counts, a computed target is skipped
	src = []byte(`{"id": "t.links", "title": "", "script": "state to = \"x.dyn\"\nhandler go() { if to == \"\" { navigate(\"x.c\") } else { navigate(to) }\n navigate(\"x.b\")\n navigate(\"x.b\") }",
	 "timelines": [], "root": {"type": "stack", "id": "root", "on": {"tap": "go()"}, "children": [{"type": "text", "id": "t", "props": {"text": "a"}, "on": {"tap": "present(\"x.a\")"}}]}}`)
	if _, m, diags = Screen(c, src, 1); len(diags) > 0 {
		t.Fatal(diags)
	}
	want = []Link{{"x.a", "present", "node:t"}, {"x.b", "navigate", "handler:go"}, {"x.c", "navigate", "handler:go"}}
	if !slices.Equal(m.Links, want) {
		t.Fatalf("links %+v, want %+v", m.Links, want)
	}
}
