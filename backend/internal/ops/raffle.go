package ops

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"
)

// Raffle serves the raffle.* operations from the NewBackend GraphQL upstream (spec 2026-09-28 §3.2).
type Raffle struct {
	G   *GQL
	Now func() time.Time // nil → time.Now
}

func (r *Raffle) Handlers() map[string]Handler {
	return map[string]Handler{
		"raffle.overview":     r.overview,
		"raffle.promo":        r.promo,
		"raffle.leaderboard":  r.leaderboard,
		"raffle.tickets":      r.tickets,
		"raffle.invites":      r.invites,
		"raffle.claimDaily":   r.claimDaily,
		"raffle.toggleHidden": r.toggleHidden,
	}
}

const (
	qRaffleOverview    = `query RaffleOverview { dailyBonusRewards { streakDay amount } currentDailyBonus { streakDay claimedToday } getMyProgressRaffle { ticketsCount hidden items { taskType name count priority progress target completed nextAvailableAt ticketsEarned } } getPrizes { mainPrize { raffleId prize { ticketsCount name imageUrl } } prizes { raffleId prize { ticketsCount name imageUrl } } } getRaffleUsersInfo(limit: 1, offset: 0) { total currentUserPosition } getRaffleTaskRules { taskType name ticketsCount description iconUrl iconUrlSmall priority target isPromo } getRaffleTime { start end } getUser { id myRefCode } }`
	qRafflePromo       = `query RafflePromo { getRaffleTime { start end } getMyProgressRaffle { ticketsCount hidden items { taskType completed } } getRaffleTaskRules { taskType name isPromo } getRaffleUsersInfo(limit: 1, offset: 0) { total currentUserPosition } }`
	qRaffleLeaderboard = `query RaffleLeaderboard($limit: Int!, $offset: Int!) { getRaffleUsersInfo(limit: $limit, offset: $offset) { total currentUserPosition items { userId ticketsCount } } getMyProgressRaffle { hidden } getUser { id firstname lastname } }`
	qRaffleUsers       = `query RaffleUsers($ids: [Int!]!) { getUsersForRaffle(userIds: $ids) { id firstname lastname } }`
	qRaffleTickets     = `query RaffleTickets($limit: Int!, $offset: Int!) { getMyTickets(limit: $limit, offset: $offset) { total items { ticketId createdAt } } }`
	qRaffleInvites     = `query RaffleInvites { getMyReferralInvites(limit: 100, offset: 0) { total maxTicketsPerInvite items { userId totalTickets } } getUser { id } }`
	qRaffleClaimDaily  = `mutation RaffleClaimDaily { claimDailyReward { streakDay } }`
	qRaffleToggle      = `mutation RaffleToggleHidden { toggleMyVisibilityInRaffle }`
)

// auth is the Authorization forwarded upstream; "" (anonymous) makes GQL use the fallback test account for reads.
func auth(req Req) string {
	if req.User == "anon" {
		return ""
	}
	return req.User
}

func (r *Raffle) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// page reads {offset, limit} args: offset ≥ 0 (GraphQL Int range), limit 1…50.
func page(args map[string]any) (offset, limit int) {
	o, _ := args["offset"].(float64)
	l, _ := args["limit"].(float64)
	return int(min(max(o, 0), math.MaxInt32)), int(min(max(l, 1), 50))
}

func (r *Raffle) overview(ctx context.Context, req Req) (Result, error) {
	var d gqlOverview
	if err := r.G.Do(ctx, auth(req), false, "RaffleOverview", qRaffleOverview, nil, &d); err != nil {
		return fail(err), nil
	}
	o, err := buildOverview(&d, r.now())
	if err != nil {
		return fail(err), nil
	}
	return OK(o), nil
}

func (r *Raffle) promo(ctx context.Context, req Req) (Result, error) {
	var d gqlOverview
	if err := r.G.Do(ctx, auth(req), false, "RafflePromo", qRafflePromo, nil, &d); err != nil {
		return fail(err), nil
	}
	return OK(buildPromo(&d, r.now())), nil
}

// names resolves "firstname lastname" of the given user ids (unique, positive; no call when there are none).
// Users without a name are left out.
func (r *Raffle) names(ctx context.Context, a string, ids []int) (map[int]string, error) {
	seen := map[int]bool{}
	var uniq []int
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	out := map[int]string{}
	if len(uniq) == 0 {
		return out, nil
	}
	var d struct {
		GetUsersForRaffle []struct {
			ID                  int
			Firstname, Lastname string
		}
	}
	if err := r.G.Do(ctx, a, false, "RaffleUsers", qRaffleUsers, map[string]any{"ids": uniq}, &d); err != nil {
		return nil, err
	}
	for _, u := range d.GetUsersForRaffle {
		if n := strings.TrimSpace(u.Firstname + " " + u.Lastname); n != "" {
			out[u.ID] = n
		}
	}
	return out, nil
}

func (r *Raffle) leaderboard(ctx context.Context, req Req) (Result, error) {
	offset, limit := page(req.Args)
	var d struct {
		GetRaffleUsersInfo struct {
			Total               int
			CurrentUserPosition *int
			Items               []struct {
				UserID       *int
				TicketsCount int
			}
		}
		GetMyProgressRaffle *struct{ Hidden bool }
		// getUsersForRaffle leaves out the current user, so the own row's name comes from getUser
		GetUser *struct{ Firstname, Lastname string }
	}
	a := auth(req)
	if err := r.G.Do(ctx, a, false, "RaffleLeaderboard", qRaffleLeaderboard, map[string]any{"limit": limit, "offset": offset}, &d); err != nil {
		return fail(err), nil
	}
	info := d.GetRaffleUsersInfo
	var ids []int
	for _, it := range info.Items {
		if it.UserID != nil {
			ids = append(ids, *it.UserID)
		}
	}
	names, err := r.names(ctx, a, ids)
	if err != nil {
		return fail(err), nil
	}
	hidden := d.GetMyProgressRaffle != nil && d.GetMyProgressRaffle.Hidden
	var myName *string
	if u := d.GetUser; u != nil {
		if n := strings.TrimSpace(u.Firstname + " " + u.Lastname); n != "" { // as in names
			myName = &n
		}
	}

	items := make([]any, 0, len(info.Items))
	for i, it := range info.Items {
		pos := offset + i + 1
		isMe := info.CurrentUserPosition != nil && pos == *info.CurrentUserPosition
		var name *string
		if it.UserID != nil {
			if n, ok := names[*it.UserID]; ok {
				name = &n
			}
		}
		if isMe && name == nil {
			name = myName
		}
		items = append(items, map[string]any{
			"key": strconv.Itoa(pos), "position": pos, "name": displayName(name, isMe, hidden),
			"tickets": it.TicketsCount, "isMe": isMe,
		})
	}
	var my any
	if p := info.CurrentUserPosition; p != nil {
		my = *p
	}
	return OK(map[string]any{"total": info.Total, "myPosition": my, "items": items}), nil
}

func (r *Raffle) tickets(ctx context.Context, req Req) (Result, error) {
	offset, limit := page(req.Args)
	var d struct {
		GetMyTickets struct {
			Total int
			Items []struct{ TicketID, CreatedAt string }
		}
	}
	if err := r.G.Do(ctx, auth(req), false, "RaffleTickets", qRaffleTickets, map[string]any{"limit": limit, "offset": offset}, &d); err != nil {
		return fail(err), nil
	}
	items := make([]any, 0, len(d.GetMyTickets.Items))
	for _, t := range d.GetMyTickets.Items {
		// The contract's createdAt is a number: an unparsable one is 0, and the ticket is kept — the screens page by
		// the item count (offset = loaded, a short page = the end), so a dropped item would stop their paging.
		at := msTime(t.CreatedAt)
		if at == nil {
			at = 0.0
		}
		items = append(items, map[string]any{"id": t.TicketID, "createdAt": at})
	}
	return OK(map[string]any{"total": d.GetMyTickets.Total, "items": items}), nil
}

func (r *Raffle) invites(ctx context.Context, req Req) (Result, error) {
	var d struct {
		GetMyReferralInvites struct {
			Total, MaxTicketsPerInvite int
			Items                      []struct{ UserID, TotalTickets int }
		}
		GetUser *struct{ ID int }
	}
	a := auth(req)
	if err := r.G.Do(ctx, a, false, "RaffleInvites", qRaffleInvites, nil, &d); err != nil {
		return fail(err), nil
	}
	inv := d.GetMyReferralInvites
	self := 0 // ids are positive
	if d.GetUser != nil {
		self = d.GetUser.ID
	}
	var ids []int
	for _, it := range inv.Items {
		ids = append(ids, it.UserID)
	}
	names, err := r.names(ctx, a, ids)
	if err != nil {
		return fail(err), nil
	}
	items := make([]any, 0, len(inv.Items))
	for _, it := range inv.Items {
		if it.UserID == self {
			continue // the upstream may list the user among their own invites
		}
		key := strconv.Itoa(it.UserID)
		name, ok := names[it.UserID]
		if !ok {
			name = key
		}
		items = append(items, map[string]any{"key": key, "name": name, "tickets": it.TotalTickets})
	}
	total := inv.Total
	if total == 0 {
		total = len(items)
	}
	return OK(map[string]any{"total": total, "maxTicketsPerInvite": inv.MaxTicketsPerInvite, "items": items}), nil
}

func (r *Raffle) claimDaily(ctx context.Context, req Req) (Result, error) {
	var d struct{ ClaimDailyReward struct{ StreakDay int } }
	if err := r.G.Do(ctx, auth(req), true, "RaffleClaimDaily", qRaffleClaimDaily, nil, &d); err != nil {
		return fail(err), nil
	}
	return OK(map[string]any{"streakDay": d.ClaimDailyReward.StreakDay}), nil
}

func (r *Raffle) toggleHidden(ctx context.Context, req Req) (Result, error) {
	var d struct{ ToggleMyVisibilityInRaffle bool }
	if err := r.G.Do(ctx, auth(req), true, "RaffleToggleHidden", qRaffleToggle, nil, &d); err != nil {
		return fail(err), nil
	}
	return OK(map[string]any{"hidden": d.ToggleMyVisibilityInRaffle}), nil
}
