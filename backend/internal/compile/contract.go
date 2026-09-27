// Package compile turns screen sources (spec §3.1) into wire documents (spec §3.8): it type-checks the
// DSL via package dsl, validates the node tree against contract/primitives.json and expands templates.
package compile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"wishbone/backend/internal/dsl"
)

// Contract is the loaded contract/ directory.
type Contract struct {
	Stdlib    *dsl.Stdlib
	Ops       map[string]*dsl.Op
	Prims     *Primitives
	Tokens    map[string]map[string]bool // color type space radius motion icon → names
	Templates map[string]*Template

	tsrc  map[string][]byte // template sources, compiled lazily while loading
	tmark map[string]uint8  // 1 compiling, 2 done
	tstk  []string
}

type Primitives struct {
	Common        map[string]*Prop
	Events        map[string]*dsl.Type // events of every node
	DragEvents    map[string]*dsl.Type // events of nodes with `drag`
	Animatable    map[string]bool
	TimelineProps map[string]bool
	Prims         map[string]*Primitive
}

type Primitive struct {
	Children bool
	Props    map[string]*Prop
	Events   map[string]*dsl.Type
	Sugar    map[string]string
}

type Prop struct {
	Type     *dsl.Type
	Required bool
}

var themeGroups = map[string]string{"colors": "color", "type": "type", "space": "space", "radius": "radius", "motion": "motion"}

// LoadContract reads dir (primitives, stdlib, operations, tokens, icons, templates). Screens are not loaded.
func LoadContract(dir string) (*Contract, error) {
	return loadContract(dir, filepath.Join(dir, "templates"))
}

func loadContract(dir, templatesDir string) (*Contract, error) {
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name)) }
	c := &Contract{Templates: map[string]*Template{}, tsrc: map[string][]byte{}, tmark: map[string]uint8{}}
	b, err := read("stdlib.json")
	if err == nil {
		c.Stdlib, err = dsl.LoadStdlib(b)
	}
	if err != nil {
		return nil, fmt.Errorf("stdlib.json: %w", err)
	}
	if b, err = read("operations.json"); err == nil {
		c.Ops, err = dsl.LoadOps(b)
	}
	if err != nil {
		return nil, fmt.Errorf("operations.json: %w", err)
	}
	if b, err = read("primitives.json"); err == nil {
		c.Prims, err = loadPrimitives(b)
	}
	if err != nil {
		return nil, fmt.Errorf("primitives.json: %w", err)
	}
	if c.Tokens, err = loadTokens(dir); err != nil {
		return nil, err
	}
	var icons map[string]json.RawMessage
	if b, err = read("icons.json"); err == nil {
		err = json.Unmarshal(b, &icons)
	}
	if err != nil {
		return nil, fmt.Errorf("icons.json: %w", err)
	}
	c.Tokens["icon"] = map[string]bool{}
	for name := range icons {
		c.Tokens["icon"][name] = true
	}
	files, _ := filepath.Glob(filepath.Join(templatesDir, "*.json"))
	for _, f := range files {
		if c.tsrc[templateName(f)], err = os.ReadFile(f); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(c.tsrc))
	for name := range c.tsrc {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := c.template(name); err != nil {
			return nil, err
		}
	}
	c.tsrc, c.tmark = nil, nil
	return c, nil
}

func templateName(file string) string { return strings.TrimSuffix(filepath.Base(file), ".json") }

func loadPrimitives(b []byte) (*Primitives, error) {
	type rawProp struct {
		Type     string
		Required bool
	}
	var raw struct {
		Common        map[string]rawProp
		Events        map[string]string
		DragEvents    map[string]string
		Animatable    []string
		TimelineProps []string
		Primitives    map[string]struct {
			Children bool
			Props    map[string]rawProp
			Events   map[string]string
			Sugar    map[string]string
		}
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	props := func(m map[string]rawProp) (map[string]*Prop, error) {
		out := map[string]*Prop{}
		for name, p := range m {
			t, err := dsl.ParseType(p.Type)
			if err != nil {
				return nil, fmt.Errorf("prop %s: %w", name, err)
			}
			out[name] = &Prop{Type: t, Required: p.Required}
		}
		return out, nil
	}
	events := func(m map[string]string) (map[string]*dsl.Type, error) {
		out := map[string]*dsl.Type{}
		for name, src := range m {
			t, err := dsl.ParseType(src)
			if err != nil {
				return nil, fmt.Errorf("event %s: %w", name, err)
			}
			out[name] = t
		}
		return out, nil
	}
	p := &Primitives{Prims: map[string]*Primitive{}, Animatable: set(raw.Animatable), TimelineProps: set(raw.TimelineProps)}
	var err error
	if p.Common, err = props(raw.Common); err != nil {
		return nil, err
	}
	if p.Events, err = events(raw.Events); err != nil {
		return nil, err
	}
	if p.DragEvents, err = events(raw.DragEvents); err != nil {
		return nil, err
	}
	for name, r := range raw.Primitives {
		prim := &Primitive{Children: r.Children, Sugar: r.Sugar}
		if prim.Props, err = props(r.Props); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if prim.Events, err = events(r.Events); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		p.Prims[name] = prim
	}
	return p, nil
}

func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// loadTokens reads every theme; all themes must define the same names per group.
func loadTokens(dir string) (map[string]map[string]bool, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "tokens", "*.json"))
	if len(files) == 0 {
		return nil, fmt.Errorf("tokens: no themes in %s", filepath.Join(dir, "tokens"))
	}
	var tokens map[string]map[string]bool
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var theme map[string]json.RawMessage
		if err := json.Unmarshal(b, &theme); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		cur := map[string]map[string]bool{}
		for key, group := range themeGroups {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(theme[key], &m); err != nil || m == nil {
				return nil, fmt.Errorf("%s: group %q missing or invalid", f, key)
			}
			cur[group] = map[string]bool{}
			for name := range m {
				cur[group][name] = true
			}
		}
		if tokens == nil {
			tokens = cur
			continue
		}
		for group, names := range cur {
			if !sameKeys(names, tokens[group]) {
				return nil, fmt.Errorf("%s: %s tokens differ from %s", f, group, filepath.Base(files[0]))
			}
		}
	}
	return tokens, nil
}

func sameKeys(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func isTemplateType(t string) bool {
	for _, r := range t {
		return unicode.IsUpper(r)
	}
	return false
}
