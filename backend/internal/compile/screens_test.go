package compile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Every shipped screen compiles against the real contract (not only at server seed time).
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

// A lazy scroll virtualizes only its direct children: a list must be a `repeat` child of it, not rows inside one
// child. One child taller than the screen after another child livelocks SwiftUI's LazyVStack, and Compose keeps the
// next child composed, so a paging `appear` stops firing (decisions/renderers.md, showcase notes).
func TestContractLazyScrollsHoldListsDirectly(t *testing.T) {
	type node struct {
		ID       string          `json:"id"`
		Type     string          `json:"type"`
		Repeat   json.RawMessage `json:"repeat"`
		Props    map[string]any  `json:"props"`
		Children []node          `json:"children"`
	}
	var nested func(n node) string // id of a repeat inside n (n itself included), "" if none
	nested = func(n node) string {
		if n.Repeat != nil {
			return n.ID
		}
		for _, c := range n.Children {
			if id := nested(c); id != "" {
				return id
			}
		}
		return ""
	}
	files, _ := filepath.Glob(filepath.Join(contractDir, "screens", "*.json"))
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var screen struct{ Root node }
		if err := json.Unmarshal(src, &screen); err != nil {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
		var walk func(n node)
		walk = func(n node) {
			if n.Type == "scroll" && n.Props["lazy"] == true {
				for _, c := range n.Children {
					if id := nested(c); c.Repeat == nil && id != "" {
						t.Errorf("%s: lazy scroll %q: repeat %q is inside child %q — make the rows children of the scroll", filepath.Base(f), n.ID, id, c.ID)
					}
				}
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(screen.Root)
	}
}
