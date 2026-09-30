package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/store"
)

type diagError []compile.Diag

func (d diagError) Error() string {
	return fmt.Sprintf("%d compile diagnostics, first: %s", len(d), d[0])
}

// builder compiles the source of screen id into a revision.
func (s *Server) builder(id string) store.Build {
	return func(src json.RawMessage, rev int) (*store.Revision, error) {
		var head struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(src, &head)
		if head.ID != id {
			return nil, diagError{{Path: "id", Msg: fmt.Sprintf("source id %q must equal the screen id %q", head.ID, id)}}
		}
		doc, m, diags := compile.Screen(s.Contract, src, rev)
		if len(diags) > 0 {
			return nil, diagError(diags)
		}
		sum := sha256.Sum256(doc)
		meta, _ := json.Marshal(newMeta(doc, m))
		return &store.Revision{Document: doc, ETag: fmt.Sprintf("%d-%x", rev, sum[:8]), RuntimeMin: m.RuntimeMin, Meta: meta}, nil
	}
}

func (s *Server) screenErr(w http.ResponseWriter, r *http.Request, err error) {
	var d diagError
	switch {
	case errors.As(err, &d):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"diags": []compile.Diag(d)})
	case errors.Is(err, store.ErrNotFound):
		s.fail(w, "SCREEN_NOT_FOUND", "screen not found")
	case errors.Is(err, store.ErrNoRevision):
		s.fail(w, "REVISION_NOT_FOUND", "revision not found")
	case errors.Is(err, store.ErrExists):
		s.fail(w, "CONFLICT", "screen id is taken")
	default:
		s.internal(w, r, err)
	}
}

type screenJSON struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Draft        json.RawMessage `json:"draft,omitempty"`
	PublishedRev *int            `json:"publishedRev"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	Unpublished  bool            `json:"unpublished"`
}

func toJSON(sc *store.Screen) screenJSON {
	return screenJSON{sc.ID, sc.Title, sc.Draft, sc.PublishedRev, sc.UpdatedAt, sc.Unpublished}
}

func (s *Server) listScreens(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Screens(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]screenJSON, len(list))
	for i := range list {
		out[i] = toJSON(&list[i])
	}
	writeJSON(w, 200, out)
}

func (s *Server) createScreen(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID, Title string }
	if !s.decode(w, r, &req) {
		return
	}
	if !screenID.MatchString(req.ID) {
		s.fail(w, "ARGS_INVALID", "id must match "+screenID.String())
		return
	}
	if req.Title == "" {
		req.Title = req.ID
	}
	sc, err := s.Store.CreateScreen(r.Context(), req.ID, req.Title, defaultDraft(req.ID, req.Title), "admin")
	if err != nil {
		s.screenErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toJSON(sc))
}

// defaultDraft is a root stack with one text.
func defaultDraft(id, title string) json.RawMessage {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	text := title
	if strings.HasPrefix(text, "=") || strings.HasPrefix(text, "@") {
		text = `\` + text
	}
	return json.RawMessage(`{"id":` + q(id) + `,"title":` + q(title) + `,"script":"","timelines":[],"root":{"type":"stack","id":"root",` +
		`"props":{"padding":16},"children":[{"type":"text","id":"text","props":{"text":` + q(text) + `}}]}}`)
}

func (s *Server) getDraft(w http.ResponseWriter, r *http.Request) {
	sc, err := s.Store.Screen(r.Context(), r.PathValue("id"))
	if err != nil {
		s.screenErr(w, r, err)
		return
	}
	writeJSON(w, 200, toJSON(sc))
}

func (s *Server) putDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title *string         `json:"title"`
		Draft json.RawMessage `json:"draft"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	if d := bytes.TrimSpace(req.Draft); len(d) == 0 || d[0] != '{' {
		s.fail(w, "ARGS_INVALID", "draft must be a JSON object")
		return
	}
	at, err := s.Store.SaveDraft(r.Context(), r.PathValue("id"), req.Title, req.Draft, "admin")
	if err != nil {
		s.screenErr(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]time.Time{"updatedAt": at})
}

func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	s.publishFrom(w, r, 0)
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	rev, err := strconv.Atoi(r.PathValue("rev"))
	if err != nil || rev < 1 {
		s.fail(w, "REVISION_NOT_FOUND", "revision not found")
		return
	}
	s.publishFrom(w, r, rev)
}

func (s *Server) publishFrom(w http.ResponseWriter, r *http.Request, from int) {
	id := r.PathValue("id")
	rev, err := s.Store.Publish(r.Context(), id, from, from > 0, "admin", s.builder(id))
	if err != nil {
		s.screenErr(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"revision": rev.Rev, "etag": rev.ETag})
}

func (s *Server) unpublish(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Unpublish(r.Context(), r.PathValue("id"), "admin"); err != nil {
		s.screenErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.Screen(r.Context(), id); err != nil {
		s.screenErr(w, r, err)
		return
	}
	revs, err := s.Store.Revisions(r.Context(), id)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	type revJSON struct {
		Revision    int       `json:"revision"`
		ETag        string    `json:"etag"`
		PublishedAt time.Time `json:"publishedAt"`
		RuntimeMin  string    `json:"runtimeMin"`
	}
	out := make([]revJSON, len(revs))
	for i, x := range revs {
		out[i] = revJSON{x.Rev, x.ETag, x.PublishedAt, x.RuntimeMin}
	}
	writeJSON(w, 200, out)
}

func (s *Server) compileSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source json.RawMessage `json:"source"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	doc, _, diags := compile.Screen(s.Contract, req.Source, 0)
	writeJSON(w, 200, map[string]any{"ok": len(diags) == 0, "diags": nonNil(diags), "document": doc})
}

func nonNil(d []compile.Diag) []compile.Diag {
	if d == nil {
		return []compile.Diag{}
	}
	return d
}

func (s *Server) template(w http.ResponseWriter, r *http.Request) {
	b, ok := s.templates[r.PathValue("name")]
	if !ok {
		s.fail(w, "NOT_FOUND", "template not found")
		return
	}
	writeRaw(w, 200, b)
}

func (s *Server) createPreview(w http.ResponseWriter, r *http.Request) {
	var req struct{ Screen string }
	if !s.decode(w, r, &req) {
		return
	}
	title := ""
	if req.Screen != "" {
		if sc, err := s.Store.Screen(r.Context(), req.Screen); err == nil {
			title = sc.Title
		}
	}
	writeJSON(w, 200, map[string]string{"code": s.Hub.Create(req.Screen, title)})
}

// previewDocument compiles a source (not stored), resolves its data and shows it in the session with its board.
func (s *Server) previewDocument(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !s.Hub.Exists(code) {
		s.fail(w, "NOT_FOUND", "preview session not found")
		return
	}
	var req struct {
		Source json.RawMessage `json:"source"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	focus, diags := s.previewScreen(r.Context(), req.Source)
	if len(diags) == 0 && !s.Hub.Push(code, s.board(r.Context(), focus)) {
		s.fail(w, "NOT_FOUND", "preview session not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": len(diags) == 0, "diags": nonNil(diags)})
}

func (s *Server) previewRetry(w http.ResponseWriter, r *http.Request) {
	if !s.Hub.Retry(r.PathValue("code")) {
		s.fail(w, "NOT_FOUND", "preview session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) previewPlay(w http.ResponseWriter, r *http.Request) {
	var req struct{ Timeline string }
	if !s.decode(w, r, &req) {
		return
	}
	if !s.Hub.Play(r.PathValue("code"), req.Timeline) {
		s.fail(w, "NOT_FOUND", "preview session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
