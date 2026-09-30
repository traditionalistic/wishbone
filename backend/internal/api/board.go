package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/preview"
	"wishbone/backend/internal/store"
)

const maxBoard = 16 // ponytail: raise when a real flow hits it

// draft is a stored draft built for boards, cached by updatedAt. Its data is resolved once, the first time it lands
// on a board. ponytail: never evicted, bounded by the number of screens.
type draft struct {
	updatedAt time.Time
	screen    preview.Screen
	meta      *revMeta // nil when the draft does not compile
}

// previewScreen compiles a source (not stored) into a preview screen and resolves its data with the param defaults.
func (s *Server) previewScreen(ctx context.Context, src json.RawMessage) (preview.Screen, []compile.Diag) {
	sc, meta, diags := s.compilePreview(src)
	if len(diags) > 0 {
		return sc, diags
	}
	sc.Data = s.previewData(ctx, meta)
	return sc, nil
}

func (s *Server) compilePreview(src json.RawMessage) (preview.Screen, *revMeta, []compile.Diag) {
	doc, m, diags := compile.Screen(s.Contract, src, 0)
	if len(diags) > 0 {
		return preview.Screen{}, nil, diags
	}
	var head struct{ ID, Title string }
	_ = json.Unmarshal(src, &head)
	meta := newMeta(doc, m)
	metaJSON, _ := json.Marshal(meta)
	return preview.Screen{ID: head.ID, Title: head.Title, Links: m.Links, Document: doc, Meta: metaJSON}, meta, nil
}

func (s *Server) previewData(ctx context.Context, meta *revMeta) json.RawMessage {
	// devices see their own host in place of preview.Base
	data, _ := s.resolve(ctx, "anon", preview.Base, meta.Data, meta.Params, nil)
	b, _ := encode(data)
	return b
}

// board is the focus and the stored drafts in its component of navigate/present links taken in either direction,
// in BFS order from the focus, at most maxBoard. A target missing from the store is a ghost. A store failure leaves
// the focus alone: the live preview never waits on the storyboard.
func (s *Server) board(ctx context.Context, focus preview.Screen) preview.Board {
	b := preview.Board{Focus: focus.ID, Screens: []preview.Screen{focus}}
	drafts, err := s.drafts(ctx, focus.ID)
	if err != nil {
		slog.Error("board: read drafts", "err", err)
		return b
	}
	in := map[string][]string{} // target → the drafts linking to it
	for _, id := range slices.Sorted(maps.Keys(drafts)) {
		for _, l := range drafts[id].screen.Links {
			in[l.To] = append(in[l.To], id)
		}
	}
	seen := map[string]bool{focus.ID: true}
	for i := 0; i < len(b.Screens) && len(b.Screens) < maxBoard; i++ {
		cur := b.Screens[i]
		var near []string
		for _, l := range cur.Links {
			near = append(near, l.To)
		}
		for _, id := range append(near, in[cur.ID]...) {
			if seen[id] || len(b.Screens) == maxBoard {
				continue
			}
			seen[id] = true
			if d, ok := drafts[id]; ok {
				b.Screens = append(b.Screens, d.screen)
			} else {
				b.Screens = append(b.Screens, preview.Screen{ID: id, Links: []compile.Link{}, Error: "screen not found"})
			}
		}
	}

	var wg sync.WaitGroup
	for i := range b.Screens[1:] {
		sc := &b.Screens[1+i]
		if d := drafts[sc.ID]; sc.Error == "" && sc.Data == nil {
			// cached until the draft changes, so a cancelled admin request must not leave failures behind
			wg.Go(func() { sc.Data = s.previewData(context.WithoutCancel(ctx), d.meta) })
		}
	}
	wg.Wait()
	s.draftMu.Lock()
	for _, sc := range b.Screens[1:] {
		if d, ok := s.draftCache[sc.ID]; ok && d.screen.Data == nil && d.updatedAt.Equal(drafts[sc.ID].updatedAt) {
			d.screen.Data = sc.Data
			s.draftCache[sc.ID] = d
		}
	}
	s.draftMu.Unlock()
	return b
}

// drafts builds every stored draft but skip (the focus comes from the admin's source), recompiling only drafts
// saved since the cache saw them.
func (s *Server) drafts(ctx context.Context, skip string) (map[string]draft, error) {
	list, err := s.Store.Screens(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]draft, len(list))
	for _, sc := range list {
		if sc.ID == skip {
			continue
		}
		s.draftMu.Lock()
		d, ok := s.draftCache[sc.ID]
		s.draftMu.Unlock()
		if !ok || !d.updatedAt.Equal(sc.UpdatedAt) {
			full, err := s.Store.Screen(ctx, sc.ID)
			if err != nil {
				return nil, err
			}
			d = s.buildDraft(full)
			s.draftMu.Lock()
			s.draftCache[sc.ID] = d
			s.draftMu.Unlock()
		}
		out[sc.ID] = d
	}
	return out, nil
}

func (s *Server) buildDraft(sc *store.Screen) draft {
	scr, meta, diags := s.compilePreview(sc.Draft)
	if len(diags) > 0 {
		scr = preview.Screen{Links: []compile.Link{}, Error: "draft does not compile: " + diags[0].Msg}
	}
	scr.ID, scr.Title = sc.ID, sc.Title
	return draft{updatedAt: sc.UpdatedAt, screen: scr, meta: meta}
}
