package api

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"wishbone/backend/internal/dsl"
)

func (s *Server) buildCatalog() ([]byte, error) {
	var prims, stdlib json.RawMessage
	var opsFile struct {
		Operations map[string]struct{ Kind, Args, Result string }
	}
	for name, v := range map[string]any{"primitives.json": &prims, "stdlib.json": &stdlib, "operations.json": &opsFile} {
		if err := readJSON(filepath.Join(s.ContractDir, name), v); err != nil {
			return nil, err
		}
	}
	type param struct {
		Name    string          `json:"name"`
		Type    string          `json:"type"`
		Default json.RawMessage `json:"default,omitempty"`
	}
	type template struct {
		Name   string   `json:"name"`
		Params []param  `json:"params"`
		Events []string `json:"events"`
		Slot   bool     `json:"slot"`
	}
	templates := []template{}
	for _, name := range sortedKeys(s.Contract.Templates) {
		t := s.Contract.Templates[name]
		defaults := templateDefaults(s.templates[name])
		params := []param{}
		for _, p := range t.Params {
			params = append(params, param{p.Name, p.Type.String(), defaults[p.Name]})
		}
		templates = append(templates, template{name, params, sortedKeys(t.Events), t.Slot})
	}
	tokens := map[string][]string{}
	for _, g := range []string{"color", "type", "space", "radius", "motion"} {
		tokens[g] = sortedKeys(s.Contract.Tokens[g])
	}
	type op struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Args   string `json:"args"`
		Result string `json:"result"`
	}
	opList := []op{}
	for _, id := range sortedKeys(opsFile.Operations) {
		o := opsFile.Operations[id]
		opList = append(opList, op{id, o.Kind, o.Args, o.Result})
	}
	return json.Marshal(struct {
		Primitives json.RawMessage     `json:"primitives"`
		Stdlib     json.RawMessage     `json:"stdlib"`
		Templates  []template          `json:"templates"`
		Tokens     map[string][]string `json:"tokens"`
		Icons      []string            `json:"icons"`
		Ops        []op                `json:"ops"`
		Brands     []string            `json:"brands"`
	}{prims, stdlib, templates, tokens, sortedKeys(s.Contract.Tokens["icon"]), opList, sortedKeys(s.themes)})
}

// templateDefaults reads param defaults from a template source in the admin's prop-value form: literals as
// JSON, tokens as "@group.name", any other expression as "=<its DSL source>".
func templateDefaults(src []byte) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	var s struct{ Script string }
	if json.Unmarshal(src, &s) != nil {
		return out
	}
	script, _ := dsl.ParseScript(s.Script)
	if script == nil {
		return out
	}
	lines := strings.Split(s.Script, "\n")
	for _, d := range script.Decls {
		p, ok := d.(*dsl.ParamDecl)
		if !ok || p.Default == nil {
			continue
		}
		v := any("=" + defaultText(lines, p.P))
		switch e := p.Default.(type) {
		case *dsl.Lit:
			v = e.Val
			if str, ok := v.(string); ok && (strings.HasPrefix(str, "=") || strings.HasPrefix(str, "@")) {
				v = `\` + str
			}
		case *dsl.TokenRef:
			v = "@" + e.Group + "." + e.Name
		case *dsl.Unary:
			if lit, ok := e.X.(*dsl.Lit); ok && e.Op == "-" {
				if n, ok := lit.Val.(float64); ok {
					v = -n
				}
			}
		}
		out[p.Name], _ = json.Marshal(v)
	}
	return out
}

// defaultText is the source after `=` on the declaration line of a param, without a trailing comment.
// ponytail: one line; a default spanning lines is cut at the first line end.
func defaultText(lines []string, at dsl.Pos) string {
	if at.Line < 1 || at.Line > len(lines) {
		return ""
	}
	line := []rune(lines[at.Line-1])
	start, end, inStr := -1, len(line), false
scan:
	for i := max(at.Col-1, 0); i < len(line); i++ {
		switch c := line[i]; {
		case inStr && c == '\\':
			i++
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '=' && start < 0:
			start = i + 1
		case c == '/' && start >= 0 && i+1 < len(line) && line[i+1] == '/':
			end = i
			break scan
		}
	}
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(string(line[start:end]))
}
