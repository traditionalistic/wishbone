package dsl

import (
	"fmt"
	"strconv"
	"strings"
)

type tokKind uint8

const (
	tEOF tokKind = iota
	tIdent
	tNum
	tStr
	tTok // @group.name, text = "group.name"
	tPunct
)

type token struct {
	kind  tokKind
	text  string
	num   float64
	parts []strPart // tStr
	pos   Pos
	nl    bool // a newline precedes the token
}

// strPart is a literal chunk, or (toks != nil) the tokens of a ${…} interpolation ending with tEOF.
type strPart struct {
	lit  string
	toks []token
	pos  Pos
}

var puncts2 = []string{"=>", "==", "!=", "<=", ">=", "&&", "||", "??"}

const puncts1 = "()[]{},:.?!=<>+-*/%|;"

type lexer struct {
	src       []rune
	i         int
	line, col int
	diags     []Diag
}

func lex(src string) ([]token, []Diag) {
	l := &lexer{src: []rune(src), line: 1, col: 1}
	return l.run(false), l.diags
}

func (l *lexer) at(k int) rune {
	if l.i+k < len(l.src) {
		return l.src[l.i+k]
	}
	return 0
}

func (l *lexer) adv() {
	if l.src[l.i] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.i++
}

func (l *lexer) pos() Pos { return Pos{l.line, l.col} }

func (l *lexer) errf(p Pos, format string, a ...any) {
	l.diags = append(l.diags, Diag{p.Line, p.Col, "syntax error: " + fmt.Sprintf(format, a...)})
}

// run lexes to the end of input or, inside an interpolation, to its closing '}'.
func (l *lexer) run(interp bool) []token {
	var toks []token
	nl, depth := false, 0
	for {
		for l.i < len(l.src) {
			c := l.at(0)
			if c == '\n' {
				nl = true
			} else if c == '/' && l.at(1) == '/' {
				for l.i < len(l.src) && l.at(0) != '\n' {
					l.adv()
				}
				continue
			} else if c != ' ' && c != '\t' && c != '\r' {
				break
			}
			l.adv()
		}
		p := l.pos()
		if l.i >= len(l.src) {
			if interp {
				l.errf(p, "unterminated interpolation")
			}
			return append(toks, token{kind: tEOF, pos: p, nl: nl})
		}
		c := l.at(0)
		var t token
		switch {
		case isIdentStart(c):
			t = token{kind: tIdent, text: l.ident()}
		case isDigit(c):
			t = l.number()
		case c == '"':
			t = l.str()
		case c == '@':
			t = l.tokenRef()
		case interp && c == '}' && depth == 0:
			l.adv()
			return append(toks, token{kind: tEOF, pos: p})
		default:
			t = l.punct()
			if t.kind == tEOF {
				l.errf(p, "unexpected character %q", c)
				l.adv()
				continue
			}
			if t.text == "{" {
				depth++
			} else if t.text == "}" {
				depth--
			}
		}
		t.pos, t.nl, nl = p, nl, false
		toks = append(toks, t)
	}
}

func isIdentStart(c rune) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c rune) bool      { return c >= '0' && c <= '9' }
func isIdentPart(c rune) bool  { return isIdentStart(c) || isDigit(c) }

func (l *lexer) ident() string {
	start := l.i
	for l.i < len(l.src) && isIdentPart(l.at(0)) {
		l.adv()
	}
	return string(l.src[start:l.i])
}

func (l *lexer) number() token {
	p, start := l.pos(), l.i
	for isDigit(l.at(0)) {
		l.adv()
	}
	if l.at(0) == '.' && isDigit(l.at(1)) {
		l.adv()
		for isDigit(l.at(0)) {
			l.adv()
		}
	}
	text, exp := string(l.src[start:l.i]), ""
	switch {
	case l.at(0) == 'm' && l.at(1) == 's' && !isIdentPart(l.at(2)):
		l.adv()
		l.adv()
	case l.at(0) == 's' && !isIdentPart(l.at(1)):
		l.adv()
		exp = "e3" // parse "1.2e3" instead of multiplying: exact for decimal inputs
	}
	if isIdentPart(l.at(0)) {
		l.errf(p, "malformed number (units are ms and s)")
		for isIdentPart(l.at(0)) {
			l.adv()
		}
	}
	v, err := strconv.ParseFloat(text+exp, 64)
	if err != nil {
		l.errf(p, "number out of range")
		v = 0
	}
	return token{kind: tNum, num: v, text: text}
}

func (l *lexer) str() token {
	p0 := l.pos()
	l.adv()
	var parts []strPart
	var sb strings.Builder
	var litPos Pos
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, strPart{lit: sb.String(), pos: litPos})
			sb.Reset()
		}
	}
	write := func(p Pos, r rune) {
		if sb.Len() == 0 {
			litPos = p
		}
		sb.WriteRune(r)
	}
	for {
		c := l.at(0)
		if l.i >= len(l.src) || c == '\n' {
			l.errf(p0, "unterminated string")
			break
		}
		p := l.pos()
		switch {
		case c == '"':
			l.adv()
			flush()
			return token{kind: tStr, parts: parts}
		case c == '\\':
			if r, ok := l.escape(); ok {
				write(p, r)
			}
		case c == '$' && l.at(1) == '{':
			flush()
			l.adv()
			l.adv()
			parts = append(parts, strPart{toks: l.run(true), pos: p})
		default:
			write(p, c)
			l.adv()
		}
	}
	flush()
	return token{kind: tStr, parts: parts}
}

func (l *lexer) escape() (rune, bool) {
	p := l.pos()
	l.adv()
	c := l.at(0)
	if l.i < len(l.src) && c != '\n' {
		l.adv()
	}
	switch c {
	case '"', '\\', '$':
		return c, true
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'u':
		if l.at(0) == '{' {
			l.adv()
			start := l.i
			for l.i < len(l.src) && l.at(0) != '}' && l.at(0) != '"' && l.at(0) != '\n' {
				l.adv()
			}
			hex := string(l.src[start:l.i])
			if l.at(0) == '}' {
				l.adv()
				v, err := strconv.ParseUint(hex, 16, 32)
				if err == nil && len(hex) <= 6 && v <= 0x10FFFF && (v < 0xD800 || v > 0xDFFF) {
					return rune(v), true
				}
			}
		}
		l.errf(p, `bad escape: expected \u{HEX} with a Unicode scalar value`)
		return 0, false
	}
	l.errf(p, `unknown escape \%c (allowed: \" \\ \n \t \$ \u{HEX})`, c)
	return 0, false
}

func (l *lexer) tokenRef() token {
	p := l.pos()
	l.adv()
	group := l.ident()
	if group == "" || l.at(0) != '.' || !isIdentStart(l.at(1)) {
		l.errf(p, "malformed token, expected @group.name")
		return token{kind: tTok, text: group + "."}
	}
	l.adv()
	return token{kind: tTok, text: group + "." + l.ident()}
}

func (l *lexer) punct() token {
	two := string([]rune{l.at(0), l.at(1)})
	for _, s := range puncts2 {
		if two == s {
			l.adv()
			l.adv()
			return token{kind: tPunct, text: s}
		}
	}
	if c := l.at(0); strings.ContainsRune(puncts1, c) {
		l.adv()
		return token{kind: tPunct, text: string(c)}
	}
	return token{kind: tEOF}
}
