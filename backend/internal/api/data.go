package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/store"
)

const (
	sourceTimeout = 500 * time.Millisecond
	dataBudget    = 1200 * time.Millisecond
)

// revMeta is stored next to each revision (and each preview document).
type revMeta struct {
	Ops    []string           `json:"ops"`    // ops used by `call`: the /v1/call allowlist
	Data   []compile.DataDecl `json:"data"`   // data sources, args with {"$param": name}
	Params map[string]any     `json:"params"` // screen param defaults
}

func newMeta(doc json.RawMessage, m compile.Meta) *revMeta {
	var d struct{ Params map[string]json.RawMessage }
	_ = json.Unmarshal(doc, &d)
	meta := &revMeta{Ops: m.Ops, Data: m.Data, Params: map[string]any{}}
	for name, raw := range d.Params {
		var wire any
		_ = json.Unmarshal(raw, &wire)
		meta.Params[name] = constant(wire)
	}
	return meta
}

// constant evaluates a param default's wire expression.
// ponytail: literals, lists, records and negation cover defaults in practice; anything else reads as null.
func constant(e any) any {
	a, _ := e.([]any)
	if len(a) < 2 {
		return nil
	}
	switch a[0] {
	case "v":
		return a[1]
	case "un":
		if len(a) > 2 && a[1] == "-" {
			if n, ok := constant(a[2]).(float64); ok {
				return -n
			}
		}
	case "list":
		xs, _ := a[1].([]any)
		out := make([]any, len(xs))
		for i, x := range xs {
			out[i] = constant(x)
		}
		return out
	case "rec":
		fs, _ := a[1].(map[string]any)
		out := map[string]any{}
		for k, x := range fs {
			out[k] = constant(x)
		}
		return out
	}
	return nil
}

// meta loads a revision's meta; revision 0 is the newest preview document of the screen.
func (s *Server) meta(ctx context.Context, screen string, rev int) (*revMeta, error) {
	var raw json.RawMessage
	if rev == 0 {
		var ok bool
		if raw, ok = s.Hub.Meta(screen); !ok {
			return nil, store.ErrNoRevision
		}
	} else {
		r, err := s.Store.Revision(ctx, screen, rev)
		if err != nil {
			return nil, err
		}
		raw = r.Meta
	}
	m := &revMeta{}
	return m, json.Unmarshal(raw, m)
}

// resolve runs data sources in parallel (500 ms each, 1200 ms in total). failed = a required source failed.
func (s *Server) resolve(ctx context.Context, usr, base string, decls []compile.DataDecl, defaults, params map[string]any) (out map[string]ops.Result, failed bool) {
	merged := map[string]any{}
	maps.Copy(merged, defaults)
	maps.Copy(merged, params)
	ctx, cancel := context.WithTimeout(ctx, dataBudget)
	defer cancel()
	res := make([]ops.Result, len(decls))
	var wg sync.WaitGroup
	for i, d := range decls {
		wg.Go(func() { res[i] = s.source(ctx, usr, base, d, merged) })
	}
	wg.Wait()
	out = map[string]ops.Result{}
	for i, d := range decls {
		out[d.Name] = res[i]
		failed = failed || (d.Required && !res[i].OK)
	}
	return out, failed
}

func (s *Server) source(ctx context.Context, usr, base string, d compile.DataDecl, params map[string]any) ops.Result {
	var raw any
	if err := json.Unmarshal(d.Args, &raw); err != nil {
		return s.sourceFailed(d, err)
	}
	args := substitute(raw, params)
	op := s.Contract.Ops[d.Op]
	if op == nil {
		return s.sourceFailed(d, errors.New("unknown op"))
	}
	if err := ops.Validate(op.Args, args); err != nil {
		return s.result(ops.Fail("ARGS_INVALID", err.Error()))
	}
	m, _ := args.(map[string]any)
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	ch := make(chan ops.Result, 1)
	go func() {
		r, err := s.invoke(ctx, d.Op, ops.Req{User: usr, Base: base, Args: m})
		if err != nil {
			r = s.sourceFailed(d, err)
		}
		ch <- r
	}()
	select {
	case r := <-ch:
		return r
	case <-ctx.Done():
		return s.sourceFailed(d, ctx.Err())
	}
}

func (s *Server) sourceFailed(d compile.DataDecl, err error) ops.Result {
	slog.Warn("data source failed", "source", d.Name, "op", d.Op, "err", err)
	return s.result(ops.Fail("DATA_SOURCE_FAILED", "Не удалось загрузить данные"))
}

// substitute replaces {"$param": name} objects with parameter values.
func substitute(v any, params map[string]any) any {
	switch v := v.(type) {
	case map[string]any:
		if name, ok := v["$param"].(string); ok && len(v) == 1 {
			return params[name]
		}
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = substitute(x, params)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = substitute(x, params)
		}
		return out
	}
	return v
}

// invoke runs a handler, turning a panic into an error, and fills error retryability from errors.json.
func (s *Server) invoke(ctx context.Context, op string, req ops.Req) (res ops.Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("op %s panicked: %v", op, p)
		}
	}()
	res, err = s.Ops[op](ctx, req)
	return s.result(res), err
}

func (s *Server) result(r ops.Result) ops.Result {
	if r.Error != nil {
		e := *r.Error
		e.Retryable = s.errs[e.Code].Retryable
		r.Error = &e
	}
	return r
}

// runtimeOK reports whether the client runtime (semver, prerelease ignored; missing = 1.0.0) is ≥ min.
func runtimeOK(have, min string) bool {
	if have == "" {
		have = "1.0.0"
	}
	h, ok1 := semver(have)
	m, ok2 := semver(min)
	if !ok1 || !ok2 {
		return false
	}
	for i := range h {
		if h[i] != m[i] {
			return h[i] > m[i]
		}
	}
	return true
}

func semver(v string) (out [3]int, ok bool) {
	v, _, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
