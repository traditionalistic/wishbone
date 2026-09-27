// Package ops is the operation registry behind `data` and `call` (contract/operations.json): Go handlers keyed
// by op id, argument validation at the trust boundary, and the showcase's demo operations.
package ops

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"wishbone/backend/internal/dsl"
)

// Req is one invocation. Args were validated against the op's args type.
type Req struct {
	User string // Authorization header value, "anon" without one; never log it
	Base string // scheme://host of the request, for absolute asset URLs
	Args map[string]any
}

// Error is the wire error {code, message, retryable}; the API fills Retryable from contract/errors.json.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Result is a data source result or a call response (contract/api.md).
type Result struct {
	OK          bool              `json:"ok"`
	Value       any               `json:"value,omitempty"`
	Error       *Error            `json:"error,omitempty"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}

func OK(v any) Result              { return Result{OK: true, Value: v} }
func Fail(code, msg string) Result { return Result{Error: &Error{Code: code, Message: msg}} }

// Handler runs an operation. A returned error is an infrastructure failure (500 / DATA_SOURCE_FAILED);
// domain failures are Results with OK = false.
type Handler func(ctx context.Context, r Req) (Result, error)

// Check verifies that handlers and contract operations match one to one (the server refuses to start otherwise).
func Check(handlers map[string]Handler, contract map[string]*dsl.Op) error {
	var missing, extra []string
	for id := range contract {
		if handlers[id] == nil {
			missing = append(missing, id)
		}
	}
	for id := range handlers {
		if contract[id] == nil {
			extra = append(extra, id)
		}
	}
	if len(missing)+len(extra) == 0 {
		return nil
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return fmt.Errorf("ops: no handler for [%s]; handler without contract op [%s]", strings.Join(missing, " "), strings.Join(extra, " "))
}

// Validate checks a decoded JSON value against a contract type (call args, substituted data args).
func Validate(t *dsl.Type, v any) error { return check(t, v, "args") }

func check(t *dsl.Type, v any, path string) error {
	bad := func() error { return fmt.Errorf("%s: expected %s", path, t) }
	switch t.Kind {
	case dsl.KAny:
	case dsl.KNull:
		if v != nil {
			return bad()
		}
	case dsl.KOpt:
		if v != nil {
			return check(t.Elem, v, path)
		}
	case dsl.KBool:
		if _, ok := v.(bool); !ok {
			return bad()
		}
	case dsl.KNumber:
		if _, ok := v.(float64); !ok {
			return bad()
		}
	case dsl.KString:
		if _, ok := v.(string); !ok {
			return bad()
		}
	case dsl.KLit:
		if s, ok := v.(string); !ok || s != t.Name {
			return bad()
		}
	case dsl.KList:
		xs, ok := v.([]any)
		if !ok {
			return bad()
		}
		for i, x := range xs {
			if err := check(t.Elem, x, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case dsl.KMap:
		m, ok := v.(map[string]any)
		if !ok {
			return bad()
		}
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if err := check(t.Elem, m[k], path+"."+k); err != nil {
				return err
			}
		}
	case dsl.KRecord:
		m, ok := v.(map[string]any)
		if !ok {
			return bad()
		}
		known := map[string]bool{}
		for _, f := range t.Fields {
			known[f.Name] = true
			x, has := m[f.Name]
			if !has || (f.Opt && x == nil) {
				if !f.Opt {
					return fmt.Errorf("%s: missing field '%s'", path, f.Name)
				}
				continue
			}
			if err := check(f.T, x, path+"."+f.Name); err != nil {
				return err
			}
		}
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !known[k] {
				return fmt.Errorf("%s: unknown field '%s'", path, k)
			}
		}
	case dsl.KUnion:
		for _, a := range t.Alts {
			if check(a, v, path) == nil {
				return nil
			}
		}
		return bad()
	default:
		return fmt.Errorf("%s: type %s is not allowed in operation arguments", path, t)
	}
	return nil
}
