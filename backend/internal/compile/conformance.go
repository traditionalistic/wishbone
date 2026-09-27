package compile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConformanceFiles are the vector files that get a compiled form (errors.json is compiler-only).
var ConformanceFiles = []string{"expr", "format", "handlers", "events", "timelines"}

// defaultEnv is the harness env of contract/README.md, in output order.
var defaultEnv = []field{
	{"now", json.RawMessage(`0`)}, {"platform", json.RawMessage(`"test"`)}, {"locale", json.RawMessage(`"ru"`)},
	{"brand", json.RawMessage(`"korpu"`)}, {"colorScheme", json.RawMessage(`"light"`)}, {"reduceMotion", json.RawMessage(`false`)},
	{"screenWidth", json.RawMessage(`390`)}, {"screenHeight", json.RawMessage(`844`)}, {"safeTop", json.RawMessage(`0`)},
	{"safeBottom", json.RawMessage(`0`)}, {"timeZone", json.RawMessage(`"UTC"`)},
}

// Conformance compiles <dir>/<file>.json into the compiled/ form of contract/README.md.
// The result is deterministic: re-running produces identical bytes.
func Conformance(c *Contract, dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, file := range ConformanceFiles {
		b, err := os.ReadFile(filepath.Join(dir, file+".json"))
		if err != nil {
			return nil, err
		}
		var vectors []json.RawMessage
		if err := json.Unmarshal(b, &vectors); err != nil {
			return nil, fmt.Errorf("%s.json: %w", file, err)
		}
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, v := range vectors {
			item, err := c.compileVector(v)
			if err != nil {
				return nil, fmt.Errorf("%s.json vector %d: %w", file, i, err)
			}
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(item)
		}
		buf.WriteByte(']')
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, buf.Bytes(), "", " "); err != nil {
			return nil, err
		}
		pretty.WriteByte('\n')
		out[file] = pretty.Bytes()
	}
	return out, nil
}

func (c *Contract) compileVector(raw json.RawMessage) ([]byte, error) {
	fields, err := orderedFields(raw)
	if err != nil {
		return nil, err
	}
	get := func(k string) json.RawMessage {
		for _, f := range fields {
			if f.k == k {
				return f.v
			}
		}
		return nil
	}
	var name, script string
	json.Unmarshal(get("name"), &name)
	if s := get("script"); s != nil {
		if err := json.Unmarshal(s, &script); err != nil {
			return nil, fmt.Errorf("%s: script: %w", name, err)
		}
	}
	root, timelines := get("root"), get("timelines")
	if root == nil {
		root = json.RawMessage(`{"type": "stack", "id": "root"}`)
	}
	if timelines == nil {
		timelines = json.RawMessage(`[]`)
	}
	src := encode(map[string]any{"id": name, "title": name, "script": script, "timelines": timelines, "root": root})
	u, doc := compileScreen(c, src, 1)
	if doc == nil {
		return nil, fmt.Errorf("%s: %v", name, u.diags)
	}
	hasEnv := false
	for i, f := range fields {
		switch f.k {
		case "expr":
			var s string
			if err := json.Unmarshal(f.v, &s); err != nil {
				return nil, fmt.Errorf("%s: expr: %w", name, err)
			}
			if fields[i].v, _ = u.expr(s, nil, nil, "expr"); len(u.diags) > 0 {
				return nil, fmt.Errorf("%s: %v", name, u.diags)
			}
		case "timeline":
			t := &srcTimeline{}
			if err := strict(f.v, t); err != nil {
				return nil, fmt.Errorf("%s: timeline: %w", name, err)
			}
			if fields[i].v = encode(u.timeline(t, "timeline", nil)); len(u.diags) > 0 {
				return nil, fmt.Errorf("%s: %v", name, u.diags)
			}
		case "env":
			hasEnv = true
			if fields[i].v, err = mergeEnv(f.v); err != nil {
				return nil, fmt.Errorf("%s: env: %w", name, err)
			}
		}
	}
	if !hasEnv {
		env, _ := mergeEnv(nil)
		fields = append(fields, field{"env", env})
	}
	fields = append(fields, field{"document", encode(doc)})
	return object(fields), nil
}

func mergeEnv(overrides json.RawMessage) (json.RawMessage, error) {
	env := append([]field{}, defaultEnv...)
	if overrides != nil {
		over, err := orderedFields(overrides)
		if err != nil {
			return nil, err
		}
	next:
		for _, o := range over {
			for i := range env {
				if env[i].k == o.k {
					env[i].v = o.v
					continue next
				}
			}
			env = append(env, o)
		}
	}
	return object(env), nil
}

type field struct {
	k string
	v json.RawMessage
}

// orderedFields decodes a JSON object keeping key order.
func orderedFields(raw json.RawMessage) ([]field, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var out []field
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, field{t.(string), v})
	}
	return out, nil
}

func object(fs []field) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encode(f.k))
		b.WriteByte(':')
		b.Write(f.v)
	}
	b.WriteByte('}')
	return b.Bytes()
}
