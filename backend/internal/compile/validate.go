package compile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"wishbone/backend/internal/dsl"
)

// value compiles a prop value (spec §3.1): "=expr", "@group.name", "\=…"/"\@…" escapes, otherwise a JSON literal.
func (u *unit) value(raw json.RawMessage, want *dsl.Type, sc *dsl.Scope, path string) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch {
		case strings.HasPrefix(s, "="):
			v, _ := u.expr(s[1:], want, sc, path)
			return v
		case strings.HasPrefix(s, "@"):
			v, _ := u.expr(s, want, sc, path)
			return v
		case strings.HasPrefix(s, `\=`), strings.HasPrefix(s, `\@`):
			raw = encode(s[1:])
		}
	}
	lit := decode(raw)
	if d := u.prog.CheckExpr(literalExpr(lit), sc, want); len(d) > 0 {
		for _, x := range d {
			u.errf(path, "%s", x.Msg)
		}
		return nil
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		panic(err)
	}
	return encode([]any{"v", json.RawMessage(b.Bytes())})
}

// literalExpr builds a DSL literal from a JSON value so it is type-checked like source text.
func literalExpr(v any) dsl.Expr {
	switch v := v.(type) {
	case json.Number:
		f, _ := v.Float64()
		return &dsl.Lit{Val: f}
	case []any:
		l := &dsl.ListLit{}
		for _, x := range v {
			l.Elems = append(l.Elems, literalExpr(x))
		}
		return l
	case map[string]any:
		r := &dsl.RecLit{}
		for _, k := range sortedKeys(v) {
			r.Fields = append(r.Fields, dsl.RecField{Name: k, X: literalExpr(v[k])})
		}
		return r
	}
	return &dsl.Lit{Val: v}
}

type dragSpec struct {
	Axis   string    `json:"axis"`
	Snap   []float64 `json:"snap,omitempty"`
	Bounds []float64 `json:"bounds,omitempty"`
}

func (u *unit) drag(raw json.RawMessage, path string) json.RawMessage {
	var d dragSpec
	if err := strict(raw, &d); err != nil {
		u.errf(path, "invalid drag: %v", err)
		return nil
	}
	switch {
	case d.Axis != "x" && d.Axis != "y" && d.Axis != "both":
		u.errf(path, `drag axis must be "x", "y" or "both"`)
	case d.Bounds != nil && (len(d.Bounds) != 2 || d.Bounds[0] > d.Bounds[1]):
		u.errf(path, "drag bounds must be [min, max]")
	default:
		return encode(d)
	}
	return nil
}

// Trans is one wire transition: {"k": "fade"}, {"k": "slide", "edge": "bottom"}, {"k": "scale", "from": 0.9}, {"k": "offset", "x": 0, "y": 20}.
type Trans struct {
	K    string   `json:"k"`
	Edge string   `json:"edge,omitempty"`
	From *float64 `json:"from,omitempty"`
	X    *float64 `json:"x,omitempty"`
	Y    *float64 `json:"y,omitempty"`
}

// parseTransition parses "fade + slide(bottom) + scale(0.9) + offset(0, 20)".
func parseTransition(s string) ([]Trans, error) {
	out := []Trans{}
	for _, part := range strings.Split(s, "+") {
		part = strings.TrimSpace(part)
		name, rest, hasArgs := strings.Cut(part, "(")
		name = strings.TrimSpace(name)
		var args []string
		if hasArgs {
			inner, ok := strings.CutSuffix(strings.TrimSpace(rest), ")")
			if !ok {
				return nil, fmt.Errorf("transition %q: missing ')'", part)
			}
			for _, a := range strings.Split(inner, ",") {
				args = append(args, strings.TrimSpace(a))
			}
		}
		nums := func(n int) ([]*float64, error) {
			if len(args) != n {
				return nil, fmt.Errorf("transition %s takes %d number(s)", name, n)
			}
			out := make([]*float64, n)
			for i, a := range args {
				f, err := strconv.ParseFloat(a, 64)
				if err != nil {
					return nil, fmt.Errorf("transition %s: %q is not a number", name, a)
				}
				out[i] = &f
			}
			return out, nil
		}
		t := Trans{K: name}
		switch name {
		case "fade":
			if hasArgs {
				return nil, fmt.Errorf("transition fade takes no arguments")
			}
		case "slide":
			if len(args) != 1 || !map[string]bool{"top": true, "bottom": true, "start": true, "end": true}[args[0]] {
				return nil, fmt.Errorf("slide edge must be top, bottom, start or end")
			}
			t.Edge = args[0]
		case "scale":
			n, err := nums(1)
			if err != nil {
				return nil, err
			}
			t.From = n[0]
		case "offset":
			n, err := nums(2)
			if err != nil {
				return nil, err
			}
			t.X, t.Y = n[0], n[1]
		case "":
			return nil, fmt.Errorf("empty transition in %q", s)
		default:
			return nil, fmt.Errorf("unknown transition '%s' (fade, slide(edge), scale(from), offset(x, y))", name)
		}
		out = append(out, t)
	}
	return out, nil
}

// --- timelines ---

type srcTimeline struct {
	Name        string     `json:"name"`
	Repeat      *float64   `json:"repeat"`
	Autoreverse bool       `json:"autoreverse"`
	Autoplay    bool       `json:"autoplay"`
	Tracks      []srcTrack `json:"tracks"`
}

type srcTrack struct {
	Target  string   `json:"target"`
	Prop    string   `json:"prop"`
	Stagger *float64 `json:"stagger"`
	Keys    []srcKey `json:"keys"`
}

type srcKey struct {
	T    *float64        `json:"t"`
	V    *float64        `json:"v"`
	Ease json.RawMessage `json:"ease"`
}

// Timeline is the wire form (spec §3.8); Duration is the largest key time.
type Timeline struct {
	Duration    float64 `json:"duration"`
	Repeat      float64 `json:"repeat"`
	Autoreverse bool    `json:"autoreverse"`
	Autoplay    bool    `json:"autoplay"`
	Tracks      []Track `json:"tracks"`
}

type Track struct {
	Target  string  `json:"target"`
	Prop    string  `json:"prop"`
	Stagger float64 `json:"stagger"`
	Keys    []Key   `json:"keys"`
}

type Key struct {
	T    float64   `json:"t"`
	V    float64   `json:"v"`
	Ease []float64 `json:"ease,omitempty"`
}

// timeline validates a source timeline; target (nil = unchecked) says whether a node id exists.
func (u *unit) timeline(t *srcTimeline, path string, target func(string) bool) *Timeline {
	w := &Timeline{Repeat: 1, Autoreverse: t.Autoreverse, Autoplay: t.Autoplay, Tracks: []Track{}}
	if t.Repeat != nil {
		w.Repeat = *t.Repeat
		if r := w.Repeat; r != math.Trunc(r) || r < 1 && r != -1 {
			u.errf(path, "timeline repeat must be an integer >= 1 or -1, got %v", r)
		}
	}
	seen := map[string]bool{}
	for j, tr := range t.Tracks {
		tp := fmt.Sprintf("%s/tracks/%d", path, j)
		key := tr.Target + "." + tr.Prop
		switch {
		case target != nil && !target(tr.Target):
			u.errf(tp, "no node with id '%s'", tr.Target)
		case !u.c.Prims.TimelineProps[tr.Prop]:
			u.errf(tp, "'%s' cannot be animated by a timeline (opacity, scale, offsetX, offsetY, rotation)", tr.Prop)
		case seen[key]:
			u.errf(tp, "duplicate track for %s", key)
		case len(tr.Keys) == 0:
			u.errf(tp, "a track needs at least one key")
		}
		seen[key] = true
		wt := Track{Target: tr.Target, Prop: tr.Prop, Keys: []Key{}}
		if tr.Stagger != nil {
			if wt.Stagger = *tr.Stagger; wt.Stagger < 0 {
				u.errf(tp, "stagger must be >= 0")
			}
		}
		for k, key := range tr.Keys {
			kp := fmt.Sprintf("%s/keys/%d", tp, k)
			if key.T == nil || *key.T < 0 || key.V == nil {
				u.errf(kp, "a key needs t >= 0 (ms) and a number v")
				continue
			}
			wk := Key{T: *key.T, V: *key.V}
			if key.Ease != nil {
				var err error
				if wk.Ease, err = u.ease(key.Ease); err != nil {
					u.errf(kp, "%v", err)
				}
			}
			w.Duration = max(w.Duration, wk.T)
			wt.Keys = append(wt.Keys, wk)
		}
		sort.SliceStable(wt.Keys, func(a, b int) bool { return wt.Keys[a].T < wt.Keys[b].T })
		w.Tracks = append(w.Tracks, wt)
	}
	return w
}

// ease resolves an easing name or [x1, y1, x2, y2]; linear is omitted on the wire.
func (u *unit) ease(raw json.RawMessage) ([]float64, error) {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		e, ok := u.c.Stdlib.Easings[name]
		switch {
		case !ok:
			return nil, fmt.Errorf("unknown ease '%s' (%s or [x1, y1, x2, y2])", name, strings.Join(sortedKeys(u.c.Stdlib.Easings), ", "))
		case name == "linear":
			return nil, nil
		}
		return e[:], nil
	}
	var b []float64
	if json.Unmarshal(raw, &b) != nil || len(b) != 4 || b[0] < 0 || b[0] > 1 || b[2] < 0 || b[2] > 1 {
		return nil, fmt.Errorf("ease must be a name or [x1, y1, x2, y2] with x1, x2 in [0, 1]")
	}
	return b, nil
}
