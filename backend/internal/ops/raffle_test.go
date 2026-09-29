package ops

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/raffle/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mergeData merges the "data" objects of GraphQL responses (later keys win).
func mergeData(t *testing.T, bodies ...string) string {
	t.Helper()
	data := map[string]any{}
	for _, b := range bodies {
		var r struct{ Data map[string]any }
		if err := json.Unmarshal([]byte(b), &r); err != nil {
			t.Fatal(err)
		}
		for k, v := range r.Data {
			data[k] = v
		}
	}
	out, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

const (
	visible = `{"data":{"getMyProgressRaffle":{"hidden":false}}}`
	me      = `{"data":{"getUser":{"id":1188,"firstname":"Ваня","lastname":"Тестов"}}}`
)

func raffleEnv(t *testing.T) (*Raffle, *upstream) {
	t.Helper()
	fx := func(name string) string { return fixture(t, name) }
	u := &upstream{t: t, bodies: map[string]string{
		"RaffleOverview":     mergeData(t, fx("overview"), fx("me")),
		"RafflePromo":        fx("overview"),
		"RaffleLeaderboard":  mergeData(t, fx("leaderboard"), visible),
		"RaffleUsers":        fx("users"),
		"RaffleTickets":      fx("tickets"),
		"RaffleInvites":      mergeData(t, fx("invites"), fx("me")),
		"RaffleClaimDaily":   `{"data":{"claimDailyReward":{"streakDay":2}}}`,
		"RaffleToggleHidden": `{"data":{"toggleMyVisibilityInRaffle":true}}`,
	}}
	ts := httptest.NewServer(u)
	t.Cleanup(ts.Close)
	return &Raffle{G: &GQL{URL: ts.URL, Login: "79956791664:133767"}, Now: func() time.Time { return fixtureNow }}, u
}

func run(t *testing.T, r *Raffle, op, user, args string) Result {
	t.Helper()
	res, err := r.Handlers()[op](context.Background(), Req{User: user, Args: val(t, args).(map[string]any)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

var raffleArgs = map[string]string{
	"raffle.overview": `{}`, "raffle.promo": `{}`, "raffle.invites": `{}`,
	"raffle.leaderboard": `{"offset": 20, "limit": 20}`, "raffle.tickets": `{"offset": 0, "limit": 20}`,
	"raffle.claimDaily": `{}`, "raffle.toggleHidden": `{}`,
}

func TestRaffleResultsMatchContract(t *testing.T) {
	r, _ := raffleEnv(t)
	ops := contractOps(t)
	for id := range r.Handlers() {
		op := ops[id]
		if op == nil {
			t.Fatalf("%s: not in the contract", id)
		}
		if err := Validate(op.Args, val(t, raffleArgs[id])); err != nil {
			t.Fatalf("%s: test args do not match contract: %v", id, err)
		}
		res := run(t, r, id, "Bearer u", raffleArgs[id])
		if !res.OK {
			t.Fatalf("%s: %+v", id, res.Error)
		}
		if err := Validate(op.Result, roundTrip(t, res.Value)); err != nil {
			t.Errorf("%s: result does not match %s: %v", id, op.Result, err)
		}
	}
}

func TestRaffleLeaderboard(t *testing.T) {
	item := func(items []any, position int) map[string]any { return items[position-1].(map[string]any) }

	t.Run("first page", func(t *testing.T) {
		r, u := raffleEnv(t)
		u.bodies["RaffleLeaderboard"] = mergeData(t, fixture(t, "overview"), visible, me)
		v := roundTrip(t, run(t, r, "raffle.leaderboard", "Bearer u", `{"offset": 0, "limit": 20}`).Value).(map[string]any)
		items := v["items"].([]any)
		if len(items) != 20 || v["total"] != 330.0 || v["myPosition"] != 11.0 {
			t.Fatalf("page: %d items, total %v, myPosition %v", len(items), v["total"], v["myPosition"])
		}
		for i, x := range items {
			if m := x.(map[string]any); m["position"] != float64(i+1) || m["key"] != strconv.Itoa(i+1) {
				t.Fatalf("row %d: %v", i, m)
			}
		}
		if m := item(items, 1); m["key"] != "1" || m["name"] != "Жужужу Тютютю" || m["tickets"] != 2530.0 || m["isMe"] != false {
			t.Fatalf("row 1: %v", m)
		}
		if m := item(items, 5); m["name"] != "**********" {
			t.Fatalf("row 5 (no userId): %v", m)
		}
		// getUsersForRaffle leaves the current user out: the own row's name comes from getUser
		if m := item(items, 11); m["isMe"] != true || m["name"] != "Я Ваня Тестов" {
			t.Fatalf("row 11 (me): %v", m)
		}
		if vars := u.vars["RaffleLeaderboard"]; vars["limit"] != 20.0 || vars["offset"] != 0.0 {
			t.Fatalf("leaderboard vars: %v", vars)
		}
		if ids := u.vars["RaffleUsers"]["ids"].([]any); len(ids) != 16 {
			t.Fatalf("names asked for %d ids, want 16 unique non-null: %v", len(ids), ids)
		}
	})

	t.Run("own name hidden or unknown", func(t *testing.T) {
		for body, want := range map[string]string{
			mergeData(t, fixture(t, "overview"), `{"data":{"getMyProgressRaffle":{"hidden":true}}}`, me): "Я **********",
			mergeData(t, fixture(t, "overview"), visible, `{"data":{"getUser":null}}`):                   "Я",
		} {
			r, u := raffleEnv(t)
			u.bodies["RaffleLeaderboard"] = body
			v := roundTrip(t, run(t, r, "raffle.leaderboard", "Bearer u", `{"offset": 0, "limit": 20}`).Value).(map[string]any)
			if m := item(v["items"].([]any), 11); m["name"] != want {
				t.Fatalf("row 11 (me): %v, want %q", m, want)
			}
		}
	})

	t.Run("second page", func(t *testing.T) {
		r, u := raffleEnv(t)
		v := roundTrip(t, run(t, r, "raffle.leaderboard", "Bearer u", `{"offset": 20, "limit": 20}`).Value).(map[string]any)
		items := v["items"].([]any)
		if len(items) != 20 || v["myPosition"] != 11.0 {
			t.Fatalf("page: %d items, myPosition %v", len(items), v["myPosition"])
		}
		for i, x := range items {
			m := x.(map[string]any)
			if m["position"] != float64(21+i) || m["isMe"] != false {
				t.Fatalf("row %d: %v", i, m)
			}
		}
		if m := items[35-20].(map[string]any); m["key"] != "36" || m["name"] != "**********" || m["tickets"] != 424.0 {
			t.Fatalf("row 36 (no userId): %v", m)
		}
		if vars := u.vars["RaffleLeaderboard"]; vars["limit"] != 20.0 || vars["offset"] != 20.0 {
			t.Fatalf("leaderboard vars: %v", vars)
		}
	})

	t.Run("page is clamped", func(t *testing.T) {
		r, u := raffleEnv(t)
		run(t, r, "raffle.leaderboard", "Bearer u", `{"offset": -5, "limit": 500}`)
		if vars := u.vars["RaffleLeaderboard"]; vars["limit"] != 50.0 || vars["offset"] != 0.0 {
			t.Fatalf("leaderboard vars: %v", vars)
		}
	})
}

func TestRaffleAnonymousReadsUseTheTestAccount(t *testing.T) {
	r, u := raffleEnv(t)
	if res := run(t, r, "raffle.promo", "anon", `{}`); !res.OK {
		t.Fatalf("anonymous promo: %+v", res.Error)
	}
	if u.logins != 1 || len(u.auth) != 2 || !strings.HasPrefix(u.auth[1], "Bearer h.") {
		t.Fatalf("logins %d, auth %v", u.logins, u.auth)
	}
	res := run(t, r, "raffle.claimDaily", "anon", `{}`)
	if res.OK || res.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("anonymous claim: %+v", res)
	}
	for _, op := range u.ops {
		if op == "RaffleClaimDaily" {
			t.Fatal("anonymous write reached the upstream")
		}
	}
}

func TestRaffleUnauthenticated(t *testing.T) {
	r, u := raffleEnv(t)
	u.bodies["RaffleOverview"] = fixture(t, "unauth")
	if res := run(t, r, "raffle.overview", "Bearer expired", `{}`); res.OK || res.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("overview: %+v", res)
	}
}

func TestRaffleInvites(t *testing.T) {
	r, u := raffleEnv(t)
	b, _ := json.Marshal(run(t, r, "raffle.invites", "Bearer u", `{}`).Value)
	if string(b) != `{"items":[],"maxTicketsPerInvite":30,"total":0}` {
		t.Fatalf("empty invites: %s", b)
	}

	// the user's own row is dropped; names come from getUsersForRaffle, else the id; total 0 → row count
	u.bodies["RaffleInvites"] = mergeData(t, fixture(t, "me"), `{"data":{"getMyReferralInvites":{"total":0,"maxTicketsPerInvite":30,"items":[
		{"userId":1188,"totalTickets":30},{"userId":1181,"totalTickets":10},{"userId":999,"totalTickets":30}]}}}`)
	b, _ = json.Marshal(run(t, r, "raffle.invites", "Bearer u", `{}`).Value)
	want := `{"items":[{"key":"1181","name":"Жужужу Тютютю","tickets":10},{"key":"999","name":"999","tickets":30}],"maxTicketsPerInvite":30,"total":2}`
	if string(b) != want {
		t.Fatalf("invites:\n got %s\nwant %s", b, want)
	}
}

// An unparsable createdAt keeps its ticket (the screens page by the item count) with the contract's number type.
func TestRaffleTicketsKeepBadTimestamps(t *testing.T) {
	r, u := raffleEnv(t)
	u.bodies["RaffleTickets"] = `{"data":{"getMyTickets":{"total":2,"items":[
		{"ticketId":"a","createdAt":"2026-09-10T15:09:41.60322Z"},{"ticketId":"b","createdAt":"10.09.2026"}]}}}`
	b, _ := json.Marshal(run(t, r, "raffle.tickets", "Bearer u", `{"offset": 0, "limit": 20}`).Value)
	if want := `{"items":[{"createdAt":1789052981603,"id":"a"},{"createdAt":0,"id":"b"}],"total":2}`; string(b) != want {
		t.Fatalf("tickets:\n got %s\nwant %s", b, want)
	}
}

func TestRaffleWrites(t *testing.T) {
	r, _ := raffleEnv(t)
	if b, _ := json.Marshal(run(t, r, "raffle.claimDaily", "Bearer u", `{}`).Value); string(b) != `{"streakDay":2}` {
		t.Fatalf("claim: %s", b)
	}
	if b, _ := json.Marshal(run(t, r, "raffle.toggleHidden", "Bearer u", `{}`).Value); string(b) != `{"hidden":true}` {
		t.Fatalf("toggle: %s", b)
	}
}
