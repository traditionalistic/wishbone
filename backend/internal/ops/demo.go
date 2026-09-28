package ops

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Demo holds the showcase operations: in-memory, deterministic, per-user state keyed by Req.User.
type Demo struct {
	NoLatency bool // tests: skip the simulated network latency

	mu      sync.Mutex
	tickets map[string]float64
	subs    int
}

func (d *Demo) Handlers() map[string]Handler {
	return map[string]Handler{
		"demo.profile":    d.profile,
		"demo.offers":     d.offers,
		"demo.plans":      d.plans,
		"demo.checkPromo": d.checkPromo,
		"demo.subscribe":  d.subscribe,
		"demo.claimOffer": d.claimOffer,
	}
}

func (d *Demo) wait(ctx context.Context, ms time.Duration) error {
	if d.NoLatency {
		return ctx.Err()
	}
	t := time.NewTimer(ms * time.Millisecond)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// addTickets adds n to the user's tickets (starting at 12) and returns the new count.
func (d *Demo) addTickets(user string, n float64) float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tickets == nil {
		d.tickets = map[string]float64{}
	}
	t, ok := d.tickets[user]
	if !ok {
		t = 12
	}
	d.tickets[user] = t + n
	return t + n
}

func (d *Demo) profile(_ context.Context, r Req) (Result, error) {
	return OK(map[string]any{"name": "Ivan", "tickets": d.addTickets(r.User, 0), "avatar": r.Base + "/assets/avatar.png", "level": 7, "authorized": r.User != "anon"}), nil
}

var offers = []struct{ title, subtitle string }{
	{"Двойные билеты", "Каждая покупка приносит вдвое больше билетов"},
	{"Кешбэк 10%", "Возвращаем часть суммы бонусами"},
	{"Ранний доступ", "Участвуйте в розыгрышах раньше всех"},
	{"Секретный приз", "Откройте, чтобы узнать, что внутри"},
	{"Неделя без комиссии", "Переводы без процентов семь дней"},
	{"Лотерея дня", "Один билет — один шанс на главный приз"},
}

func (d *Demo) offers(_ context.Context, r Req) (Result, error) {
	now := time.Now().UnixMilli()
	out := make([]map[string]any, len(offers))
	for i, o := range offers {
		n := i + 1
		out[i] = map[string]any{
			"id": fmt.Sprintf("o%d", n), "title": o.title, "subtitle": o.subtitle, "price": 49 * n,
			"image": fmt.Sprintf("%s/assets/offers/o%d.jpg", r.Base, n), "until": now + int64(n)*3600*1000,
		}
	}
	return OK(out), nil
}

var plans = []struct {
	id, title string
	monthly   float64
	perks     []string
}{
	{"basic", "Базовый", 199, []string{"1 билет в неделю", "Участие в общих розыгрышах"}},
	{"pro", "Про", 499, []string{"5 билетов в неделю", "Эксклюзивные розыгрыши", "Кешбэк 5%"}},
	{"max", "Макс", 999, []string{"Безлимит билетов", "Все розыгрыши", "Кешбэк 10%", "Личный менеджер"}},
}

func (d *Demo) plans(context.Context, Req) (Result, error) {
	out := make([]map[string]any, len(plans))
	for i, p := range plans {
		out[i] = map[string]any{"id": p.id, "title": p.title, "monthly": p.monthly, "perks": p.perks}
	}
	return OK(out), nil
}

var promos = map[string]float64{"WB10": 10, "WB25": 25}

func promo(code string) float64 { return promos[strings.ToUpper(strings.TrimSpace(code))] }

func (d *Demo) checkPromo(ctx context.Context, r Req) (Result, error) {
	if err := d.wait(ctx, 300); err != nil {
		return Result{}, err
	}
	if p := promo(r.Args["code"].(string)); p > 0 {
		return OK(map[string]any{"valid": true, "discount": p, "message": fmt.Sprintf("Скидка %g%% применена", p)}), nil
	}
	return OK(map[string]any{"valid": false, "discount": 0, "message": "Промокод не найден"}), nil
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func (d *Demo) subscribe(ctx context.Context, r Req) (Result, error) {
	if err := d.wait(ctx, 600); err != nil {
		return Result{}, err
	}
	email := strings.TrimSpace(r.Args["email"].(string))
	fe := map[string]string{}
	switch {
	case !emailRe.MatchString(email):
		fe["email"] = "Введите корректный email"
	case strings.EqualFold(email, "taken@example.com"):
		fe["email"] = "Этот email уже занят"
	}
	if strings.TrimSpace(r.Args["name"].(string)) == "" {
		fe["name"] = "Как к вам обращаться?"
	}
	var price float64
	for _, p := range plans {
		if p.id == r.Args["plan"] {
			price = p.monthly
		}
	}
	if price == 0 {
		fe["plan"] = "Выберите тариф"
	}
	if len(fe) > 0 {
		res := Fail("ARGS_INVALID", "Проверьте поля формы")
		res.FieldErrors = fe
		return res, nil
	}
	d.mu.Lock()
	d.subs++
	id := fmt.Sprintf("sub_%d", d.subs)
	d.mu.Unlock()
	total := math.Round(price*(100-promo(r.Args["promo"].(string)))) / 100
	return OK(map[string]any{"id": id, "total": total}), nil
}

func (d *Demo) claimOffer(_ context.Context, r Req) (Result, error) {
	return OK(map[string]any{"tickets": d.addTickets(r.User, 3)}), nil
}
