package ops

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Wire shapes of the raffle GraphQL operations this file assembles (spec 2026-09-28 §3.2). Field sets are
// deliberately narrow: only what buildOverview/buildPromo read.

type gqlTask struct {
	TaskType        string  `json:"taskType"`
	Name            string  `json:"name"`
	Count           int     `json:"count"`
	Priority        int     `json:"priority"`
	Progress        int     `json:"progress"`
	Target          *int    `json:"target"`
	Completed       bool    `json:"completed"`
	NextAvailableAt *string `json:"nextAvailableAt"`
	TicketsEarned   int     `json:"ticketsEarned"`
}

type gqlRule struct {
	TaskType     string `json:"taskType"`
	Name         string `json:"name"`
	TicketsCount int    `json:"ticketsCount"`
	Description  string `json:"description"`
	IconURL      string `json:"iconUrl"`
	IconURLSmall string `json:"iconUrlSmall"`
	Priority     int    `json:"priority"`
	Target       *int   `json:"target"`
	IsPromo      bool   `json:"isPromo"`
}

type gqlPrize struct {
	RaffleID string `json:"raffleId"`
	Prize    struct {
		TicketsCount int    `json:"ticketsCount"`
		Name         string `json:"name"`
		ImageURL     string `json:"imageUrl"`
	} `json:"prize"`
}

type gqlOverview struct {
	DailyBonusRewards []struct{ StreakDay, Amount int } `json:"dailyBonusRewards"`
	CurrentDailyBonus struct {
		StreakDay    int  `json:"streakDay"`
		ClaimedToday bool `json:"claimedToday"`
	} `json:"currentDailyBonus"`
	GetMyProgressRaffle *struct {
		TicketsCount int       `json:"ticketsCount"`
		Hidden       bool      `json:"hidden"`
		Items        []gqlTask `json:"items"`
	} `json:"getMyProgressRaffle"`
	GetPrizes *struct {
		MainPrize *gqlPrize  `json:"mainPrize"`
		Prizes    []gqlPrize `json:"prizes"`
	} `json:"getPrizes"`
	GetRaffleUsersInfo *struct {
		Total               int  `json:"total"`
		CurrentUserPosition *int `json:"currentUserPosition"`
	} `json:"getRaffleUsersInfo"`
	GetRaffleTaskRules []gqlRule                    `json:"getRaffleTaskRules"`
	GetRaffleTime      *struct{ Start, End string } `json:"getRaffleTime"`
	GetUser            *struct {
		MyRefCode *string `json:"myRefCode"`
	} `json:"getUser"`
}

const masked = "**********"

// shareBody is the fixed pitch below the referral link (Stratoapp RaffleShare.xcstrings).
const shareBody = "Залетай по моей ссылке, скачивай приложение 📲 и участвуй в розыгрыше!\n\n" +
	"Среди призов — автомобиль Geely Coolray 🚗, iPhone 17 Pro Max 📱, MacBook 💻, PlayStation 5 🎮 и многое другое 🎁✨"

func shareText(code string) string {
	return "📱 Скачай приложение: " + code + "\n\n" + shareBody
}

// displayName masks a leaderboard entry's name (Stratoapp RaffleOverviewAssembler.maskedName, collapsed to
// the isMe/hidden inputs the server has).
func displayName(name *string, isMe, hidden bool) string {
	if !isMe {
		if name != nil {
			return *name
		}
		return masked
	}
	if name == nil {
		if hidden {
			return masked
		}
		return "Я"
	}
	if hidden {
		return "Я " + masked
	}
	return "Я " + *name
}

// grouped renders an int with thousands separated by a space, e.g. 50427 → "50 427" (Stratoapp
// RaffleOverviewAssembler.grouped).
func grouped(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	groups := []string{}
	for len(s) > 3 {
		groups = append([]string{s[len(s)-3:]}, groups...)
		s = s[:len(s)-3]
	}
	groups = append([]string{s}, groups...)
	out := strings.Join(groups, " ")
	if neg {
		out = "-" + out
	}
	return out
}

// parseISO parses an RFC 3339 timestamp (with or without a fractional part); "" or an unparsable value is
// reported via ok=false.
func parseISO(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// msTime converts an RFC 3339 timestamp to JSON-friendly epoch milliseconds, or nil if it is empty/unparsable.
func msTime(iso string) any {
	if t, ok := parseISO(iso); ok {
		return float64(t.UnixMilli())
	}
	return nil
}

// trimmed reports a whitespace-trimmed string and whether it is non-empty.
func trimmed(s string) (string, bool) {
	t := strings.TrimSpace(s)
	return t, t != ""
}

// trimmedOrNil is trimmed's value as an any, for direct use as a map value ("" → nil).
func trimmedOrNil(s string) any {
	if t, ok := trimmed(s); ok {
		return t
	}
	return nil
}

// ruleIcon picks a rule/task icon URL: the small variant first, then the large one, trimmed, "" → nil.
func ruleIcon(small, large string) any {
	if t, ok := trimmed(small); ok {
		return t
	}
	return trimmedOrNil(large)
}

// dedupeRules drops rules that repeat an earlier (isPromo, taskType, name), keeping the first occurrence
// (Stratoapp RaffleOverviewAssembler.deduplicatedRules).
func dedupeRules(rules []gqlRule) []gqlRule {
	seen := map[string]bool{}
	out := make([]gqlRule, 0, len(rules))
	for _, r := range rules {
		key := strconv.FormatBool(r.IsPromo) + "|" + r.TaskType + "|" + r.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func progressRow(label, value string, ticket bool) map[string]any {
	return map[string]any{"label": label, "value": value, "ticket": ticket}
}

// ticketsRow is the "Получено билетов:" row, shown only once tickets were actually earned.
func ticketsRow(ticketsEarned int) []any {
	if ticketsEarned <= 0 {
		return []any{}
	}
	return []any{progressRow("Получено билетов:", grouped(ticketsEarned), true)}
}

// progressRows builds a task's progress rows (Stratoapp RaffleOverviewAssembler.progressItems/ticketsReceivedItems).
func progressRows(taskType string, hasItems, hasCooldown bool, count, progress, ticketsEarned int, target *int) []any {
	if hasCooldown {
		return ticketsRow(ticketsEarned)
	}
	if !hasItems {
		return []any{}
	}
	switch taskType {
	case "FRIEND_STEPS_RATING", "SELF_STEPS_GOAL":
		if target == nil || *target <= 0 {
			return []any{}
		}
		rows := []any{progressRow("Пройдено шагов:", grouped(progress), false)}
		if *target > progress {
			rows = append(rows, progressRow("Осталось:", grouped(*target-progress), false))
		}
		return rows
	case "SELF_HOUR_IN_APP":
		rows := []any{progressRow("Проведено часов:", grouped(count), false)}
		return append(rows, ticketsRow(ticketsEarned)...)
	}
	if target != nil && *target > 1 {
		done := count
		if done > *target {
			done = *target
		}
		rows := []any{progressRow("Выполнено:", grouped(done)+"/"+grouped(*target), false)}
		return append(rows, ticketsRow(ticketsEarned)...)
	}
	if count > 0 || ticketsEarned > 0 {
		rows := []any{progressRow("Выполнено:", grouped(count), false)}
		return append(rows, ticketsRow(ticketsEarned)...)
	}
	return []any{}
}

// buildTasks joins progress items onto the active (non-promo, deduped) rules, sorted by priority desc
// (Stratoapp RaffleOverviewAssembler.tasks).
func buildTasks(d *gqlOverview, now time.Time) []any {
	var items []gqlTask
	if d.GetMyProgressRaffle != nil {
		items = d.GetMyProgressRaffle.Items
	}
	byType := map[string][]gqlTask{}
	for _, it := range items {
		byType[it.TaskType] = append(byType[it.TaskType], it)
	}

	active := make([]gqlRule, 0, len(d.GetRaffleTaskRules))
	for _, r := range dedupeRules(d.GetRaffleTaskRules) {
		if !r.IsPromo {
			active = append(active, r)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].Priority > active[j].Priority })

	out := make([]any, 0, len(active))
	for _, rule := range active {
		its := byType[rule.TaskType]

		var ticketsEarned, count, progress int
		var cooldown *time.Time
		for _, it := range its {
			ticketsEarned += it.TicketsEarned
			count += it.Count
			if it.Progress > progress {
				progress = it.Progress
			}
			if it.NextAvailableAt != nil {
				if t, ok := parseISO(*it.NextAvailableAt); ok {
					if cooldown == nil || t.After(*cooldown) {
						tt := t
						cooldown = &tt
					}
				}
			}
		}

		target := rule.Target
		if len(its) > 0 && its[0].Target != nil {
			target = its[0].Target
		}

		var cooldownUntil any
		hasCooldown := false
		if cooldown != nil && cooldown.After(now) {
			cooldownUntil = float64(cooldown.UnixMilli())
			hasCooldown = true
		}

		out = append(out, map[string]any{
			"id":            rule.TaskType,
			"name":          rule.Name,
			"description":   rule.Description,
			"icon":          ruleIcon(rule.IconURLSmall, rule.IconURL),
			"tickets":       float64(rule.TicketsCount),
			"cooldownUntil": cooldownUntil,
			"progress":      progressRows(rule.TaskType, len(its) > 0, hasCooldown, count, progress, ticketsEarned, target),
		})
	}
	return out
}

// buildRules lists the promo rules (deduped, upstream order) as the promo banner sees them.
func buildRules(rules []gqlRule) []any {
	out := []any{}
	for _, r := range dedupeRules(rules) {
		if !r.IsPromo {
			continue
		}
		out = append(out, map[string]any{
			"name":        r.Name,
			"description": r.Description,
			"icon":        ruleIcon(r.IconURLSmall, r.IconURL),
			"tickets":     float64(r.TicketsCount),
		})
	}
	return out
}

// summary counts active (non-promo) task types and how many of them the user has completed (Stratoapp
// RaffleOverviewAssembler.taskSummary). Unlike buildTasks/buildRules this reads the raw rule list: a Set
// of task types already collapses duplicates, so no (isPromo, taskType, name) dedup is needed here.
func summary(d *gqlOverview) (done, total int) {
	active := map[string]bool{}
	for _, r := range d.GetRaffleTaskRules {
		if !r.IsPromo {
			active[r.TaskType] = true
		}
	}
	var items []gqlTask
	if d.GetMyProgressRaffle != nil {
		items = d.GetMyProgressRaffle.Items
	}
	completed := map[string]bool{}
	for _, it := range items {
		if it.Completed {
			completed[it.TaskType] = true
		}
	}
	total = len(active)
	for t := range completed {
		if active[t] {
			done++
		}
	}
	return
}

// buildPrizes ranks the prizes of the (lexicographically first) raffle id, deduping repeated images unless
// the prize name contains "coin" (Stratoapp RaffleOverviewAssembler.prizes).
func buildPrizes(items []gqlPrize) []any {
	groups := map[string][]gqlPrize{}
	for _, it := range items {
		groups[it.RaffleID] = append(groups[it.RaffleID], it)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	primary := items
	if len(keys) > 0 {
		primary = groups[keys[0]]
	}

	primary = append([]gqlPrize(nil), primary...)
	sort.SliceStable(primary, func(i, j int) bool {
		a, b := primary[i].Prize, primary[j].Prize
		if a.TicketsCount != b.TicketsCount {
			return a.TicketsCount > b.TicketsCount
		}
		return a.Name < b.Name
	})

	seen := map[string]bool{}
	out := make([]any, 0, len(primary))
	for i, p := range primary {
		isCoin := strings.Contains(strings.ToLower(p.Prize.Name), "coin")
		var image any
		if t, ok := trimmed(p.Prize.ImageURL); ok {
			switch {
			case isCoin:
				image = t
			case !seen[t]:
				seen[t] = true
				image = t
			}
		}
		out = append(out, map[string]any{
			"rank":    float64(i + 1),
			"title":   p.Prize.Name,
			"tickets": float64(p.Prize.TicketsCount),
			"image":   image,
		})
	}
	return out
}

// buildDaily assembles the 7-day streak strip and the amount claimable today (Stratoapp
// RaffleOverviewAssembler.dailyRewards).
func buildDaily(d *gqlOverview) map[string]any {
	rewards := append([]struct{ StreakDay, Amount int }(nil), d.DailyBonusRewards...)
	sort.SliceStable(rewards, func(i, j int) bool { return rewards[i].StreakDay < rewards[j].StreakDay })

	streakDay := d.CurrentDailyBonus.StreakDay
	claimedToday := d.CurrentDailyBonus.ClaimedToday

	days := make([]any, 0, len(rewards))
	var claimAmount any
	for _, r := range rewards {
		var state string
		switch {
		case r.StreakDay < streakDay:
			state = "claimed"
		case r.StreakDay == streakDay:
			if claimedToday {
				state = "claimed"
			} else {
				state = "available"
				claimAmount = float64(r.Amount)
			}
		default:
			state = "waiting"
		}
		days = append(days, map[string]any{"day": float64(r.StreakDay), "amount": float64(r.Amount), "state": state})
	}
	return map[string]any{"days": days, "claimAmount": claimAmount}
}

// buildOverview assembles the full raffle screen payload. It fails when a part the client can't render
// without is missing from the upstream response.
func buildOverview(d *gqlOverview, now time.Time) (map[string]any, error) {
	if d.GetMyProgressRaffle == nil || d.GetPrizes == nil || d.GetRaffleUsersInfo == nil {
		return nil, &failure{"UPSTREAM_FAILED", msgUpstream}
	}

	var place any
	if p := d.GetRaffleUsersInfo.CurrentUserPosition; p != nil {
		place = float64(*p)
	}

	var startsAt, endsAt any
	if d.GetRaffleTime != nil {
		startsAt = msTime(d.GetRaffleTime.Start)
		endsAt = msTime(d.GetRaffleTime.End)
	}

	var mainPrize any
	if mp := d.GetPrizes.MainPrize; mp != nil {
		mainPrize = map[string]any{
			"title": mp.Prize.Name,
			"image": trimmedOrNil(mp.Prize.ImageURL),
		}
	}

	var share any
	if d.GetUser != nil && d.GetUser.MyRefCode != nil && *d.GetUser.MyRefCode != "" {
		share = shareText(*d.GetUser.MyRefCode)
	}

	done, total := summary(d)

	return map[string]any{
		"tickets":      float64(d.GetMyProgressRaffle.TicketsCount),
		"hidden":       d.GetMyProgressRaffle.Hidden,
		"place":        place,
		"participants": float64(d.GetRaffleUsersInfo.Total),
		"startsAt":     startsAt,
		"endsAt":       endsAt,
		"daily":        buildDaily(d),
		"mainPrize":    mainPrize,
		"prizes":       buildPrizes(d.GetPrizes.Prizes),
		"tasks":        buildTasks(d, now),
		"tasksDone":    float64(done),
		"tasksTotal":   float64(total),
		"rules":        buildRules(d.GetRaffleTaskRules),
		"share":        share,
	}, nil
}

// buildPromo assembles the feed promo card: raffle-active flag, end time, the user's place and task
// progress (Stratoapp RaffleResponseMapper.feedPromo, extended with taskSummary).
func buildPromo(d *gqlOverview, now time.Time) map[string]any {
	active := false
	var endsAt any
	if d.GetRaffleTime != nil {
		if start, okS := parseISO(d.GetRaffleTime.Start); okS {
			if end, okE := parseISO(d.GetRaffleTime.End); okE {
				active = !now.Before(start) && now.Before(end)
				endsAt = float64(end.UnixMilli())
			}
		}
	}

	var place any
	if d.GetRaffleUsersInfo != nil {
		if p := d.GetRaffleUsersInfo.CurrentUserPosition; p != nil {
			place = float64(*p)
		}
	}

	done, total := summary(d)

	return map[string]any{
		"active":     active,
		"endsAt":     endsAt,
		"place":      place,
		"tasksDone":  float64(done),
		"tasksTotal": float64(total),
	}
}
