package ops

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"wishbone/backend/internal/dsl"
)

func contractOps(t *testing.T) map[string]*dsl.Op {
	t.Helper()
	b, err := os.ReadFile("../../../contract/operations.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := dsl.LoadOps(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func typ(t *testing.T, src string) *dsl.Type {
	t.Helper()
	ty, err := dsl.ParseType(src)
	if err != nil {
		t.Fatal(err)
	}
	return ty
}

func val(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValidate(t *testing.T) {
	cases := []struct {
		typ, val string
		err      string // "" = valid
	}{
		{"{}", `{}`, ""},
		{"{}", `{"x": 1}`, "unknown field 'x'"},
		{"{code: string}", `{"code": "WB10"}`, ""},
		{"{code: string}", `{"code": 10}`, "args.code: expected string"},
		{"{code: string}", `{}`, "missing field 'code'"},
		{"{code: string}", `[]`, "expected {code: string}"},
		{"{a?: number}", `{}`, ""},
		{"{a?: number}", `{"a": null}`, ""},
		{"{a?: number}", `{"a": "1"}`, "args.a: expected number"},
		{"{a: number?}", `{"a": null}`, ""},
		{"{a: bool}", `{"a": true}`, ""},
		{"{a: bool}", `{"a": 1}`, "expected bool"},
		{"{xs: list<number>}", `{"xs": [1, 2]}`, ""},
		{"{xs: list<number>}", `{"xs": [1, "2"]}`, "args.xs[1]: expected number"},
		{"{m: map<string>}", `{"m": {"a": "b"}}`, ""},
		{"{m: map<string>}", `{"m": {"a": 1}}`, "args.m.a: expected string"},
		{`{s: "a" | "b"}`, `{"s": "b"}`, ""},
		{`{s: "a" | "b"}`, `{"s": "c"}`, "expected"},
		{`{s: number | "fill"}`, `{"s": "fill"}`, ""},
		{`{s: number | "fill"}`, `{"s": 3}`, ""},
		{"{r: {x: number}}", `{"r": {"x": 1}}`, ""},
		{"{r: {x: number}}", `{"r": {"x": 1, "y": 2}}`, "args.r: unknown field 'y'"},
		{"{a: any}", `{"a": [1, {"b": null}]}`, ""},
		{"{c: color}", `{"c": "#fff"}`, "not allowed"},
	}
	for _, c := range cases {
		err := Validate(typ(t, c.typ), val(t, c.val))
		switch {
		case c.err == "" && err != nil:
			t.Errorf("%s ← %s: unexpected error %v", c.typ, c.val, err)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s ← %s: error %v, want containing %q", c.typ, c.val, err, c.err)
		}
	}
}

func TestCheckCoversContract(t *testing.T) {
	d := &Demo{NoLatency: true}
	all := func() map[string]Handler {
		h := d.Handlers()
		maps.Copy(h, (&Raffle{G: &GQL{}}).Handlers())
		return h
	}
	if err := Check(all(), contractOps(t)); err != nil {
		t.Fatal(err)
	}
	h := all()
	delete(h, "demo.plans")
	if err := Check(h, contractOps(t)); err == nil || !strings.Contains(err.Error(), "demo.plans") {
		t.Fatalf("missing handler must be reported: %v", err)
	}
	h = all()
	h["demo.nope"] = h["demo.plans"]
	if err := Check(h, contractOps(t)); err == nil || !strings.Contains(err.Error(), "demo.nope") {
		t.Fatalf("extra handler must be reported: %v", err)
	}
}

func call(t *testing.T, d *Demo, op, user, args string) Result {
	t.Helper()
	r, err := d.Handlers()[op](context.Background(), Req{User: user, Base: "http://h:1", Args: val(t, args).(map[string]any)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Every demo handler's ok value conforms to the op's result type from operations.json.
func TestDemoResultsMatchContract(t *testing.T) {
	d := &Demo{NoLatency: true}
	ops := contractOps(t)
	args := map[string]string{
		"demo.profile": `{}`, "demo.offers": `{}`, "demo.plans": `{}`,
		"demo.checkPromo": `{"code": "WB10"}`,
		"demo.subscribe":  `{"email": "a@b.co", "name": "Ivan", "plan": "pro", "promo": ""}`,
		"demo.claimOffer": `{"id": "o1"}`,
	}
	for id, op := range ops {
		if !strings.HasPrefix(id, "demo.") {
			continue // raffle.* is checked by TestRaffleResultsMatchContract
		}
		if err := Validate(op.Args, val(t, args[id])); err != nil {
			t.Fatalf("%s: test args do not match contract: %v", id, err)
		}
		r := call(t, d, id, "u", args[id])
		if !r.OK {
			t.Fatalf("%s: %+v", id, r.Error)
		}
		if err := Validate(op.Result, roundTrip(t, r.Value)); err != nil {
			t.Errorf("%s: result does not match %s: %v", id, op.Result, err)
		}
	}
}

func roundTrip(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return val(t, string(b))
}

func TestDemoProfileAndClaim(t *testing.T) {
	d := &Demo{NoLatency: true}
	p := roundTrip(t, call(t, d, "demo.profile", "alice", `{}`).Value).(map[string]any)
	if p["name"] != "Ivan" || p["tickets"] != 12.0 || p["level"] != 7.0 || p["avatar"] != "http://h:1/assets/avatar.png" || p["authorized"] != true {
		t.Fatalf("profile %v", p)
	}
	c := roundTrip(t, call(t, d, "demo.claimOffer", "alice", `{"id": "o2"}`).Value).(map[string]any)
	if c["tickets"] != 15.0 {
		t.Fatalf("claim %v", c)
	}
	if p := roundTrip(t, call(t, d, "demo.profile", "alice", `{}`).Value).(map[string]any); p["tickets"] != 15.0 {
		t.Fatalf("tickets not persisted: %v", p)
	}
	if p := roundTrip(t, call(t, d, "demo.profile", "bob", `{}`).Value).(map[string]any); p["tickets"] != 12.0 {
		t.Fatalf("state leaked between users: %v", p)
	}
	if p := roundTrip(t, call(t, d, "demo.profile", "anon", `{}`).Value).(map[string]any); p["authorized"] != false {
		t.Fatalf("anonymous profile reports authorized: %v", p)
	}
}

func TestDemoOffersAndPlans(t *testing.T) {
	d := &Demo{NoLatency: true}
	offers := roundTrip(t, call(t, d, "demo.offers", "u", `{}`).Value).([]any)
	if len(offers) != 6 {
		t.Fatalf("offers %d", len(offers))
	}
	o3 := offers[2].(map[string]any)
	if o3["id"] != "o3" || o3["image"] != "http://h:1/assets/offers/o3.jpg" || o3["title"] == "" {
		t.Fatalf("offer %v", o3)
	}
	if u1, u3 := offers[0].(map[string]any)["until"].(float64), o3["until"].(float64); u3-u1 != 2*3600*1000 {
		t.Fatalf("until must be now + N hours: %v %v", u1, u3)
	}
	plans := roundTrip(t, call(t, d, "demo.plans", "u", `{}`).Value).([]any)
	want := map[string]float64{"basic": 199, "pro": 499, "max": 999}
	for _, p := range plans {
		p := p.(map[string]any)
		if want[p["id"].(string)] != p["monthly"] {
			t.Errorf("plan %v", p)
		}
		if n := len(p["perks"].([]any)); n < 2 || n > 4 {
			t.Errorf("plan %v perks %d", p["id"], n)
		}
	}
	if len(plans) != 3 {
		t.Fatalf("plans %d", len(plans))
	}
}

func TestDemoCheckPromo(t *testing.T) {
	d := &Demo{NoLatency: true}
	for code, want := range map[string]string{
		"WB10": `{"discount":10,"message":"Скидка 10% применена","valid":true}`,
		"wb25": `{"discount":25,"message":"Скидка 25% применена","valid":true}`,
		"NOPE": `{"discount":0,"message":"Промокод не найден","valid":false}`,
	} {
		b, _ := json.Marshal(call(t, d, "demo.checkPromo", "u", `{"code": "`+code+`"}`).Value)
		if string(b) != want {
			t.Errorf("%s: %s, want %s", code, b, want)
		}
	}
}

func TestDemoSubscribe(t *testing.T) {
	d := &Demo{NoLatency: true}
	fail := func(args string, field, msg string) {
		t.Helper()
		r := call(t, d, "demo.subscribe", "u", args)
		if r.OK || r.Error == nil || r.Error.Code != "ARGS_INVALID" || r.FieldErrors[field] != msg {
			t.Errorf("%s: %+v %v", args, r.Error, r.FieldErrors)
		}
	}
	fail(`{"email": "nope", "name": "I", "plan": "pro", "promo": ""}`, "email", "Введите корректный email")
	fail(`{"email": "taken@example.com", "name": "I", "plan": "pro", "promo": ""}`, "email", "Этот email уже занят")
	fail(`{"email": "a@b.co", "name": "  ", "plan": "pro", "promo": ""}`, "name", "Как к вам обращаться?")
	fail(`{"email": "a@b.co", "name": "I", "plan": "gold", "promo": ""}`, "plan", "Выберите тариф")

	ok := func(args, want string) {
		t.Helper()
		r := call(t, d, "demo.subscribe", "u", args)
		b, _ := json.Marshal(r.Value)
		if !r.OK || string(b) != want {
			t.Errorf("%s: %+v %s, want %s", args, r.Error, b, want)
		}
	}
	ok(`{"email": "a@b.co", "name": "I", "plan": "pro", "promo": ""}`, `{"id":"sub_1","total":499}`)
	ok(`{"email": "a@b.co", "name": "I", "plan": "basic", "promo": "WB10"}`, `{"id":"sub_2","total":179.1}`)
	ok(`{"email": "a@b.co", "name": "I", "plan": "max", "promo": "wb25"}`, `{"id":"sub_3","total":749.25}`)
}

func TestDemoLatencyHonoursContext(t *testing.T) {
	d := &Demo{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Handlers()["demo.checkPromo"](ctx, Req{Args: map[string]any{"code": "x"}}); err == nil {
		t.Fatal("a cancelled context must abort the simulated latency")
	}
}
