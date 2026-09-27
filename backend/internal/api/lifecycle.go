package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/store"
)

// Seed creates and publishes every contract/screens/*.json whose id is not in the database yet.
func (s *Server) Seed(ctx context.Context) error {
	files, _ := filepath.Glob(filepath.Join(s.ContractDir, "screens", "*.json"))
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var head struct{ ID, Title string }
		if err := json.Unmarshal(src, &head); err != nil || !screenID.MatchString(head.ID) {
			return fmt.Errorf("seed %s: invalid source or id %q: %v", f, head.ID, err)
		}
		if _, err := s.Store.Screen(ctx, head.ID); !errors.Is(err, store.ErrNotFound) {
			if err != nil {
				return err
			}
			continue
		}
		if _, _, diags := compile.Screen(s.Contract, src, 1); len(diags) > 0 {
			return fmt.Errorf("seed %s: %w", f, diagError(diags))
		}
		if _, err := s.Store.CreateScreen(ctx, head.ID, head.Title, src, "seed"); err != nil {
			return err
		}
		if _, err := s.Store.Publish(ctx, head.ID, 0, false, "seed", s.builder(head.ID)); err != nil {
			return err
		}
		slog.Info("seeded screen", "screen", head.ID)
	}
	return nil
}

// Recompile rebuilds every live revision against the current contract (templates may have changed): a
// changed document is published as a new revision; a source that no longer compiles is an error (spec §2.3).
func (s *Server) Recompile(ctx context.Context) error {
	screens, err := s.Store.Screens(ctx)
	if err != nil {
		return err
	}
	var broken []string
	for _, sc := range screens {
		if sc.PublishedRev == nil {
			continue
		}
		live, err := s.Store.Live(ctx, sc.ID)
		if err != nil {
			return err
		}
		doc, _, diags := compile.Screen(s.Contract, live.Source, live.Rev)
		if len(diags) > 0 {
			broken = append(broken, fmt.Sprintf("%s rev %d: %v", sc.ID, live.Rev, diagError(diags)))
			continue
		}
		if bytes.Equal(doc, live.Document) {
			continue
		}
		r, err := s.Store.Publish(ctx, sc.ID, live.Rev, false, "system", s.builder(sc.ID))
		if err != nil {
			return err
		}
		slog.Info("republished after contract change", "screen", sc.ID, "revision", r.Rev)
	}
	if len(broken) > 0 {
		return fmt.Errorf("published screens no longer compile: %s", strings.Join(broken, "; "))
	}
	return nil
}
