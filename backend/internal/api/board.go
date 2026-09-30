package api

import (
	"context"
	"encoding/json"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/preview"
)

// previewScreen compiles a source (not stored) into a preview screen and resolves its data with the param defaults.
func (s *Server) previewScreen(ctx context.Context, src json.RawMessage) (preview.Screen, []compile.Diag) {
	doc, m, diags := compile.Screen(s.Contract, src, 0)
	if len(diags) > 0 {
		return preview.Screen{}, diags
	}
	var head struct{ ID, Title string }
	_ = json.Unmarshal(src, &head)
	meta := newMeta(doc, m)
	// devices see their own host in place of preview.Base
	data, _ := s.resolve(ctx, "anon", preview.Base, meta.Data, meta.Params, nil)
	dataJSON, _ := encode(data)
	metaJSON, _ := json.Marshal(meta)
	return preview.Screen{ID: head.ID, Title: head.Title, Links: m.Links, Document: doc, Data: dataJSON, Meta: metaJSON}, nil
}
