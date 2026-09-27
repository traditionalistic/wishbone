package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

type vector struct {
	Name      string
	Script    string
	Expr      string
	Error     string
	Timelines []struct{ Name string }
}

func loadVectors(t *testing.T, file string) []vector {
	t.Helper()
	var vs []vector
	if err := json.Unmarshal(readContract(t, "conformance/"+file), &vs); err != nil {
		t.Fatal(err)
	}
	return vs
}

// skipVectors: vectors that contradict stdlib.json semantics (reported, never edited here).
var skipVectors = map[string]string{}

func vectorEnv(t *testing.T, v vector) *Env {
	env := contractEnv(t)
	env.Timelines = map[string]bool{}
	for _, tl := range v.Timelines {
		env.Timelines[tl.Name] = true
	}
	return env
}

func TestConformanceCompiles(t *testing.T) {
	for _, file := range []string{"expr.json", "format.json", "handlers.json"} {
		for _, v := range loadVectors(t, file) {
			t.Run(v.Name, func(t *testing.T) {
				if why, ok := skipVectors[v.Name]; ok {
					t.Skip(why)
				}
				e, prog, diags := compile(t, vectorEnv(t, v), v.Script, v.Expr, nil)
				if len(diags) > 0 {
					t.Fatalf("script %q expr %q: %v", v.Script, v.Expr, diags)
				}
				wire := [][]byte{mustJSON(t, prog.Lower(nil))}
				if e != nil {
					wire = append(wire, LowerExpr(e, nil))
				}
				for _, w := range wire {
					if !json.Valid(w) {
						t.Fatalf("invalid wire JSON: %s", w)
					}
				}
			})
		}
	}
}

func TestConformanceErrors(t *testing.T) {
	for _, v := range loadVectors(t, "errors.json") {
		t.Run(v.Name, func(t *testing.T) {
			if why, ok := skipVectors[v.Name]; ok {
				t.Skip(why)
			}
			_, _, diags := compile(t, vectorEnv(t, v), v.Script, v.Expr, nil)
			if !hasDiag(diags, v.Error) {
				t.Fatalf("script %q expr %q: diags %v, want one containing %q", v.Script, v.Expr, diags, strings.ToLower(v.Error))
			}
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FuzzCompile: arbitrary admin input must yield diagnostics, never a panic; clean input must lower.
func FuzzCompile(f *testing.F) {
	for _, file := range []string{"expr.json", "handlers.json", "errors.json"} {
		var vs []vector
		if err := json.Unmarshal(readContract(f, "conformance/"+file), &vs); err != nil {
			f.Fatal(err)
		}
		for _, v := range vs {
			f.Add(v.Script, v.Expr)
		}
	}
	env := contractEnv(f)
	env.Timelines = map[string]bool{"intro": true}
	f.Fuzz(func(t *testing.T, script, expr string) {
		s, diags := ParseScript(script)
		prog, d := Check(s, env)
		if len(diags)+len(d) == 0 {
			prog.Lower(nil)
		}
		e, diags := ParseExpr(expr)
		if e != nil && len(diags) == 0 && len(prog.CheckExpr(e, &Scope{Self: true}, nil)) == 0 && len(d) == 0 {
			LowerExpr(e, nil)
		}
		if ss, diags := ParseStmts(expr); len(diags) == 0 && len(prog.CheckStmts(ss, nil)) == 0 && len(d) == 0 {
			LowerStmts(ss, nil)
		}
	})
}
