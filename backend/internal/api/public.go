package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/store"
)

func (s *Server) getScreen(w http.ResponseWriter, r *http.Request) {
	var params map[string]any
	if p := r.URL.Query().Get("params"); p != "" {
		if err := json.Unmarshal([]byte(p), &params); err != nil {
			s.fail(w, "ARGS_INVALID", "params must be a JSON object")
			return
		}
	}
	rev, err := s.Store.Live(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		s.fail(w, "SCREEN_NOT_FOUND", "Экран не найден")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if !runtimeOK(r.Header.Get("X-Wishbone-Runtime"), rev.RuntimeMin) {
		s.fail(w, "UNSUPPORTED_RUNTIME", "Обновите приложение, чтобы открыть этот экран")
		return
	}
	var m revMeta
	if err := json.Unmarshal(rev.Meta, &m); err != nil {
		s.internal(w, r, err)
		return
	}
	data, failed := s.resolve(r.Context(), user(r), base(r), m.Data, m.Params, params)
	if failed {
		s.fail(w, "DATA_REQUIRED_FAILED", "Не удалось загрузить данные")
		return
	}
	doc := rev.Document
	if etagMatch(r.Header.Get("If-None-Match"), rev.ETag) {
		doc = nil
	}
	writeJSON(w, 200, struct {
		ETag     string                `json:"etag"`
		Revision int                   `json:"revision"`
		Document json.RawMessage       `json:"document"`
		Data     map[string]ops.Result `json:"data"`
	}{rev.ETag, rev.Rev, doc, data})
}

func (s *Server) revisionErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNoRevision) {
		s.fail(w, "REVISION_NOT_FOUND", "Экран обновился, загрузите его заново")
		return
	}
	s.internal(w, r, err)
}

func (s *Server) postData(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Screen   string         `json:"screen"`
		Revision int            `json:"revision"`
		Params   map[string]any `json:"params"`
		Sources  []string       `json:"sources"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	m, err := s.meta(r.Context(), req.Screen, req.Revision)
	if err != nil {
		s.revisionErr(w, r, err)
		return
	}
	decls := m.Data
	if len(req.Sources) > 0 {
		decls = nil
		for _, name := range req.Sources {
			i := slices.IndexFunc(m.Data, func(d compile.DataDecl) bool { return d.Name == name })
			if i < 0 {
				s.fail(w, "ARGS_INVALID", "unknown data source '"+name+"'")
				return
			}
			decls = append(decls, m.Data[i])
		}
	}
	data, _ := s.resolve(r.Context(), user(r), base(r), decls, m.Params, req.Params)
	writeJSON(w, 200, data)
}

func (s *Server) postCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Screen   string          `json:"screen"`
		Revision int             `json:"revision"`
		Op       string          `json:"op"`
		Args     json.RawMessage `json:"args"`
		Key      string          `json:"idempotencyKey"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	op := s.Contract.Ops[req.Op]
	if op == nil {
		s.fail(w, "OPERATION_UNKNOWN", "unknown operation '"+req.Op+"'")
		return
	}
	m, err := s.meta(r.Context(), req.Screen, req.Revision)
	if err != nil {
		s.revisionErr(w, r, err)
		return
	}
	if !slices.Contains(m.Ops, req.Op) {
		s.fail(w, "OPERATION_NOT_BOUND", "operation '"+req.Op+"' is not used by this revision")
		return
	}
	var args any = map[string]any{}
	if len(req.Args) > 0 && string(req.Args) != "null" {
		_ = json.Unmarshal(req.Args, &args) // already valid JSON
	}
	argMap, isObj := args.(map[string]any)
	if err := ops.Validate(op.Args, args); err != nil || !isObj {
		msg := "args must be an object"
		if err != nil {
			msg = err.Error()
		}
		s.fail(w, "ARGS_INVALID", msg)
		return
	}
	call := ops.Req{User: user(r), Base: base(r), Args: argMap}
	if op.Kind != "write" {
		res, err := s.invoke(r.Context(), req.Op, call)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		writeJSON(w, 200, res)
		return
	}

	if req.Key == "" {
		s.fail(w, "IDEMPOTENCY_REQUIRED", "write operations need an idempotencyKey")
		return
	}
	canon, _ := json.Marshal(args) // sorted keys
	hash := sha256.Sum256(append([]byte(call.User+"\x00"), canon...))
	done, err := s.Store.Reserve(r.Context(), req.Key, req.Op, hash[:])
	switch {
	case errors.Is(err, store.ErrMismatch):
		s.fail(w, "IDEMPOTENCY_MISMATCH", "idempotencyKey was used with other arguments")
		return
	case errors.Is(err, store.ErrInFlight):
		s.fail(w, "CALL_IN_FLIGHT", "Запрос уже выполняется")
		return
	case err != nil:
		s.internal(w, r, err)
		return
	case done != nil:
		writeRaw(w, 200, done)
		return
	}
	// the write completes and is recorded even if the client goes away
	ctx := context.WithoutCancel(r.Context())
	res, err := s.invoke(ctx, req.Op, call)
	if err != nil {
		if rerr := s.Store.Release(ctx, req.Key, req.Op); rerr != nil {
			slog.Error("release idempotency key", "op", req.Op, "err", rerr)
		}
		s.internal(w, r, err)
		return
	}
	body, _ := json.Marshal(res)
	if err := s.Store.Complete(ctx, req.Key, req.Op, body); err != nil {
		slog.Error("store call response", "op", req.Op, "err", err)
	}
	writeRaw(w, 200, body)
}

func (s *Server) getTheme(w http.ResponseWriter, r *http.Request) {
	t, ok := s.themes[r.URL.Query().Get("brand")]
	if !ok {
		t = s.themes["korpu"]
	}
	w.Header().Set("ETag", `"`+t.version+`"`)
	if etagMatch(r.Header.Get("If-None-Match"), t.version) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeRaw(w, 200, t.body)
}
