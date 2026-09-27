package dsl

import (
	"fmt"
	"strings"
	"testing"
)

func dumpToks(toks []token) string {
	var out []string
	for _, t := range toks {
		var s string
		switch t.kind {
		case tEOF:
			continue
		case tIdent, tPunct:
			s = t.text
		case tNum:
			s = fmt.Sprintf("#%v", t.num)
		case tTok:
			s = "@" + t.text
		case tStr:
			var ps []string
			for _, p := range t.parts {
				if p.toks == nil {
					ps = append(ps, fmt.Sprintf("%q", p.lit))
				} else {
					ps = append(ps, "${"+dumpToks(p.toks)+"}")
				}
			}
			s = "str(" + strings.Join(ps, " ") + ")"
		}
		if t.nl {
			s = `\n` + s
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func TestLexer(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a + 12 * 1.5", "a + #12 * #1.5"},
		{"300ms + 1.2s + 2s", "#300 + #1200 + #2000"},
		{"x=>x", "x => x"},
		{"a ?? b != c && d || !e <= f >= g == h", "a ?? b != c && d || ! e <= f >= g == h"},
		{"@color.bg1 @space.lg", "@color.bg1 @space.lg"},
		{`"Hi, ${name}!"`, `str("Hi, " ${name} "!")`},
		{`"a\"b\\c\n\t\u{1F600}"`, `str("a\"b\\c\n\t😀")`},
		{`"\${x}"`, `str("${x}")`},
		{`"${ {a: "x"}.a }"`, `str(${{ a : str("x") } . a})`},
		{`""`, `str()`},
		{"a // comment\nb", `a \nb`},
		{"a;b", "a ; b"},
		{"list<number>?", "list < number > ?"},
	}
	for _, c := range cases {
		toks, diags := lex(c.src)
		if len(diags) > 0 {
			t.Errorf("%q: unexpected diags %v", c.src, diags)
			continue
		}
		if got := dumpToks(toks); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestLexerPositions(t *testing.T) {
	toks, _ := lex("a\n  bb \"é${c}\"")
	want := []Pos{{1, 1}, {2, 3}, {2, 6}}
	for i, p := range want {
		if toks[i].pos != p {
			t.Errorf("token %d at %v, want %v", i, toks[i].pos, p)
		}
	}
	if c := toks[2].parts[1].toks[0]; c.pos != (Pos{2, 10}) {
		t.Errorf("interpolated token at %v, want 2:10", c.pos)
	}
}

func TestLexerErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`"abc`, "unterminated string"},
		{"\"ab\ncd\"", "unterminated string"},
		{`"\d"`, "escape"},
		{`"\u{110000}"`, "escape"},
		{"12px", "number"},
		{"a # b", "unexpected character"},
		{"@color", "token"},
		{`"${a"`, "unterminated"},
		{"2" + strings.Repeat("0", 400), "out of range"},
	}
	for _, c := range cases {
		_, diags := lex(c.src)
		if len(diags) == 0 || !strings.Contains(diags[0].Msg, c.want) || !strings.Contains(diags[0].Msg, "syntax error") {
			t.Errorf("%q: diags %v, want one containing %q", c.src, diags, c.want)
		}
	}
}
