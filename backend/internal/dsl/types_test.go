package dsl

import (
	"strings"
	"testing"
	"time"
)

func mustType(t *testing.T, src string) *Type {
	t.Helper()
	ty, err := ParseType(src)
	if err != nil {
		t.Fatalf("ParseType(%q): %v", src, err)
	}
	return ty
}

func TestParseTypeString(t *testing.T) {
	cases := []struct{ src, want string }{
		{"number", "number"},
		{"list<{id: number, qty?: string}>?", "list<{id: number, qty?: string}>?"},
		{`"a" | "b"`, `"a" | "b"`},
		{"map<bool | number>", "map<bool | number>"},
		{`number | "fill" | "wrap"`, `number | "fill" | "wrap"`},
		{"(string?)?", "string?"},
		{"number | null", "number?"},
		{"null | number? | string", "(number | string)?"},
		{`"a" | "a"`, `"a"`},
		{"null", "null"},
		{`("a" | "b")?`, `("a" | "b")?`},
		{"{}", "{}"},
		{"list<list<number>>", "list<list<number>>"},
		{"map<any>", "map<any>"},
		{"color | paint", "color | paint"},
	}
	for _, c := range cases {
		if got := mustType(t, c.src).String(); got != c.want {
			t.Errorf("%q: got %s, want %s", c.src, got, c.want)
		}
	}
}

func TestParseTypeErrors(t *testing.T) {
	for _, src := range []string{"lst<number>", "list<number", "{a: }", `"${x}"`, "number number", "{a: number, a: string}", ""} {
		if _, err := ParseType(src); err == nil {
			t.Errorf("%q: expected an error", src)
		}
	}
}

func TestLoadOpsTimeout(t *testing.T) {
	m, err := LoadOps([]byte(`{"operations": {"a.x": {"kind": "read", "args": "{}", "result": "number", "timeoutMs": 3000}, "a.y": {"kind": "read", "args": "{}", "result": "number"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["a.x"].Timeout != 3*time.Second || m["a.y"].Timeout != 0 {
		t.Fatalf("timeouts: %v %v", m["a.x"].Timeout, m["a.y"].Timeout)
	}
}

func TestAssignable(t *testing.T) {
	cases := []struct {
		src, dst string
		want     bool
	}{
		{"color", "paint", true},
		{"paint", "color", false},
		{"null", "number?", true},
		{"null", "number", false},
		{"number", "number?", true},
		{"number?", "number", false},
		{"number?", "number?", true},
		{`"a"`, `"a" | "b"`, true},
		{`"c"`, `"a" | "b"`, false},
		{"string", `"a" | "b"`, false},
		{`"a" | "b"`, "string", true},
		{`"a"`, "string", true},
		{`number | "fill"`, `number | "fill" | "wrap"`, true},
		{`"fill"`, `number | "fill"`, true},
		{"{a: number}", "{a: number, b?: string}", true},
		{"{a: number, c: bool}", "{a: number}", false},
		{"{b?: string}", "{b: string}", false},
		{"{}", "{a?: number}", true},
		{"{a: number}", "map<number>", true},
		{"{a: string}", "map<number>", false},
		{"{a: color}", "map<any>", true},
		{"map<number>", "{a: number}", false},
		{"list<color>", "list<paint>", true},
		{"list<number?>", "list<number>", false},
		{"number", "any", true},
		{"any", "number", false},
		{"textStyle", "motion", false},
		{"{a: {b: number}}", "{a: {b: number?}}", true},
	}
	for _, c := range cases {
		if got := Assignable(mustType(t, c.src), mustType(t, c.dst)); got != c.want {
			t.Errorf("Assignable(%s, %s) = %v, want %v", c.src, c.dst, got, c.want)
		}
	}
}

func TestJoin(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"number", "number", "number"},
		{"null", "number", "number?"},
		{"number", "null", "number?"},
		{"number?", "number", "number?"},
		{"color", "paint", "paint"},
		{"{a: number}", "{a: number}", "{a: number}"},
		{"number", "string", ""},
		{"list<number>", "list<string>", ""},
		{"number?", "string", ""},
	}
	for _, c := range cases {
		got := join(mustType(t, c.a), mustType(t, c.b))
		if (got == nil) != (c.want == "") || (got != nil && got.String() != c.want) {
			t.Errorf("join(%s, %s) = %v, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestWhyNot(t *testing.T) {
	cases := []struct{ src, dst, want string }{
		{"{code: string, nope: number}", "{code: string}", "unknown field 'nope'"},
		{"{one: string}", "{one?: string, other: string}", "missing field 'other'"},
		{"{code: number}", "{code: string}", "field 'code'"},
	}
	for _, c := range cases {
		if got := whyNot(mustType(t, c.src), mustType(t, c.dst)); !strings.Contains(got, c.want) {
			t.Errorf("whyNot(%s, %s) = %q, want %q", c.src, c.dst, got, c.want)
		}
	}
}
