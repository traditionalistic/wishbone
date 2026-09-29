package ops

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func loadOverview(t *testing.T) *gqlOverview {
	t.Helper()
	b, err := os.ReadFile("testdata/raffle/overview.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data gqlOverview }
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	me, _ := os.ReadFile("testdata/raffle/me.json")
	var m struct{ Data gqlOverview }
	_ = json.Unmarshal(me, &m)
	env.Data.GetUser = m.Data.GetUser
	return &env.Data
}

var fixtureNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func TestBuildOverview(t *testing.T) {
	o, err := buildOverview(loadOverview(t), fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if o["tickets"] != 1250.0 || o["place"] != 11.0 || o["participants"] != 330.0 || o["hidden"] != false {
		t.Fatalf("header: %v %v %v %v", o["tickets"], o["place"], o["participants"], o["hidden"])
	}
	if o["endsAt"] != float64(time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC).UnixMilli()) {
		t.Fatalf("endsAt: %v", o["endsAt"])
	}
	daily := o["daily"].(map[string]any)
	days := daily["days"].([]any)
	if len(days) != 7 || days[0].(map[string]any)["state"] != "available" || days[1].(map[string]any)["state"] != "waiting" || daily["claimAmount"] != 2.0 {
		t.Fatalf("daily: %v", daily)
	}
	if daily["streakDay"] != 1.0 || daily["claimedToday"] != false {
		t.Fatalf("daily streak: %v", daily)
	}
	if mp := o["mainPrize"].(map[string]any); mp["title"] != "Автомобиль" {
		t.Fatalf("main prize: %v", mp)
	}
	prizes := o["prizes"].([]any)
	first := prizes[0].(map[string]any)
	if len(prizes) != 16 || first["rank"] != 1.0 || first["title"] != "Автомобиль" || first["tickets"] != 12000.0 {
		t.Fatalf("prizes: %d %v", len(prizes), first)
	}
	// Acoin prizes share one image and keep it ("coin" exception); nothing else repeats in this fixture
	if prizes[15].(map[string]any)["image"] == nil {
		t.Fatal("coin prizes keep their shared image")
	}
	tasks := o["tasks"].([]any)
	if len(tasks) != 15 || tasks[0].(map[string]any)["id"] != "SELF_EVENT_CREATED" {
		t.Fatalf("tasks: %d %v", len(tasks), tasks[0])
	}
	if o["tasksDone"] != 5.0 || o["tasksTotal"] != 15.0 || len(o["rules"].([]any)) != 6 {
		t.Fatalf("summary: %v/%v rules %d", o["tasksDone"], o["tasksTotal"], len(o["rules"].([]any)))
	}
	if !strings.HasPrefix(o["share"].(string), "📱 Скачай приложение: https://korpu.app/ref?uid=1188\n\n") {
		t.Fatalf("share: %q", o["share"])
	}
}

func TestTaskProgressRows(t *testing.T) {
	o, _ := buildOverview(loadOverview(t), fixtureNow)
	byID := map[string][]any{}
	for _, x := range o["tasks"].([]any) {
		m := x.(map[string]any)
		byID[m["id"].(string)] = m["progress"].([]any)
	}
	row := func(id string, i int) map[string]any { return byID[id][i].(map[string]any) }
	// hours task: hours + tickets received
	if r := row("SELF_HOUR_IN_APP", 0); r["label"] != "Проведено часов:" || r["value"] != "100" {
		t.Fatalf("hours: %v", r)
	}
	if r := row("SELF_HOUR_IN_APP", 1); r["label"] != "Получено билетов:" || r["value"] != "1 000" || r["ticket"] != true {
		t.Fatalf("hours tickets: %v", r)
	}
	// steps task: walked + remaining, grouped by thousands
	if r := row("SELF_STEPS_GOAL", 0); r["label"] != "Пройдено шагов:" || r["value"] != "50 427" {
		t.Fatalf("steps: %v", r)
	}
	if r := row("SELF_STEPS_GOAL", 1); r["label"] != "Осталось:" || r["value"] != "149 573" {
		t.Fatalf("remaining: %v", r)
	}
	// target > 1: "a/b", no tickets row while nothing earned
	if len(byID["SELF_COMMENT_POSTED"]) != 1 || row("SELF_COMMENT_POSTED", 0)["value"] != "1/5" {
		t.Fatalf("comment: %v", byID["SELF_COMMENT_POSTED"])
	}
	// nothing done yet and no target: no rows
	if len(byID["SELF_EVENT_CREATED"]) != 0 {
		t.Fatalf("event: %v", byID["SELF_EVENT_CREATED"])
	}
}

func TestDisplayName(t *testing.T) {
	n := "Иван Петров"
	cases := []struct {
		name         *string
		isMe, hidden bool
		want         string
	}{
		{&n, false, false, "Иван Петров"},
		{nil, false, false, "**********"},
		{&n, true, false, "Я Иван Петров"},
		{&n, true, true, "Я **********"},
		{nil, true, false, "Я"},
		{nil, true, true, "**********"},
	}
	for _, c := range cases {
		if got := displayName(c.name, c.isMe, c.hidden); got != c.want {
			t.Errorf("displayName(%v, %v, %v) = %q, want %q", c.name, c.isMe, c.hidden, got, c.want)
		}
	}
}

func TestGroupedAndTime(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1 000", 149573: "149 573", -1200: "-1 200", 1000000: "1 000 000"} {
		if got := grouped(n); got != want {
			t.Errorf("grouped(%d) = %q", n, got)
		}
	}
	if msTime("2026-09-10T15:09:41.60322Z") != float64(time.Date(2026, 9, 10, 15, 9, 41, 603220000, time.UTC).UnixMilli()) || msTime("") != nil || msTime("nope") != nil {
		t.Fatal("msTime")
	}
}

func TestBuildPromo(t *testing.T) {
	d := loadOverview(t)
	p := buildPromo(d, fixtureNow)
	if p["active"] != true || p["place"] != 11.0 || p["tasksDone"] != 5.0 || p["tasksTotal"] != 15.0 {
		t.Fatalf("promo: %v", p)
	}
	if buildPromo(d, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))["active"] != false {
		t.Fatal("inactive after the end")
	}
	d.GetRaffleTime = nil
	if buildPromo(d, fixtureNow)["active"] != false {
		t.Fatal("no raffle time → inactive")
	}
}
