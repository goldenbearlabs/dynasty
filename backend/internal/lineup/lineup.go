// Package lineup is who starts. A lineup takes effect on a sports day and
// stays in force until a later one replaces it, so nobody has to set one
// every day. Only starters score.
package lineup

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Entry puts one player in one starting slot.
type Entry struct {
	Slot     string      `json:"slot"`
	PlayerID pgtype.UUID `json:"player_id"`
	// CountsFrom picks the game that counts, in a slot that counts only
	// some games a week: the day of that game. Empty leaves it as it was,
	// or takes the player's next game.
	CountsFrom string `json:"counts_from"`
}

// View is a franchise's lineup for one day or week: the slots to fill, and
// every main-roster player with where he is, his games and whether he can
// still be moved.
type View struct {
	Day     string          `json:"day"`      // first day this lineup applies to
	LastDay string          `json:"last_day"` // the same day, or the end of the week
	Slots   []settings.Slot `json:"slots"`
	Players []Player        `json:"players"`
	// Locked is why the whole lineup can no longer change, or empty.
	Locked string `json:"locked"`
}

type Player struct {
	StarterEligible bool        `json:"starter_eligible"`
	EligibilityNote string      `json:"eligibility_note"`
	PlayerID        pgtype.UUID `json:"player_id"`
	FullName        string      `json:"full_name"`
	Positions       []string    `json:"positions"`
	HeadshotURL     string      `json:"headshot_url"`
	TeamAbbrev      string      `json:"team_abbrev"`
	Slot            string      `json:"slot"`        // empty on the bench
	CountsFrom      string      `json:"counts_from"` // the day his counted games begin, when his slot counts only some
	Locked          bool        `json:"locked"`
	// Points over the day or week, whether he started or not. For a starter
	// in a slot that counts only some games, only the games that count.
	Points float64 `json:"points"`
	Games  []Game  `json:"games"`
}

type Game struct {
	Day      string    `json:"day"`
	StartsAt time.Time `json:"starts_at"`
	Status   string    `json:"status"`
	Opponent string    `json:"opponent"`
	AtHome   bool      `json:"at_home"`
	Points   float64   `json:"points"`
	Counts   bool      `json:"counts"` // false for a game his slot leaves out
}

// Get returns the lineup in force on a day.
func (s *Service) Get(ctx context.Context, league db.League, franchiseID pgtype.UUID, day time.Time) (View, error) {
	return load(ctx, db.New(s.pool), league, franchiseID, day)
}

// span is the run of days one lineup covers: the day itself, or its week.
func span(rules settings.Lineup, day time.Time) (first, last time.Time) {
	if rules.Period == settings.PeriodWeek {
		first = sportsday.WeekStart(day, rules.WeekStart)
		return first, first.AddDate(0, 0, 6)
	}
	return day, day
}

func load(ctx context.Context, q *db.Queries, league db.League, franchiseID pgtype.UUID, day time.Time) (View, error) {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return View{}, err
	}
	first, last := span(rules.Lineup, day)
	view := View{
		Day: first.Format(time.DateOnly), LastDay: last.Format(time.DateOnly),
		Slots: rules.Lineup.Slots, Players: []Player{},
	}
	now := time.Now()
	switch {
	case last.Before(sportsday.Today()):
		view.Locked = "This lineup is in the past."
	case rules.Lineup.Lock == settings.LockPeriodStart && !now.Before(sportsday.Start(first)):
		view.Locked = "This lineup locked when its " + rules.Lineup.Period + " began."
	}

	// Who is starting, from the lineup in force.
	starting := map[pgtype.UUID]string{}
	countsFrom := map[pgtype.UUID]string{}
	inForce, err := q.LineupInForce(ctx, db.LineupInForceParams{LeagueID: league.ID, FranchiseID: franchiseID, Day: sportsday.Date(first)})
	if err != nil {
		return View{}, err
	}
	if inForce.Valid {
		entries, err := q.ListLineupEntries(ctx, db.ListLineupEntriesParams{LeagueID: league.ID, FranchiseID: franchiseID, EffectiveOn: inForce})
		if err != nil {
			return View{}, err
		}
		for _, e := range entries {
			starting[e.PlayerID] = e.Slot
			if e.CountsFrom.Valid {
				countsFrom[e.PlayerID] = e.CountsFrom.Time.Format(time.DateOnly)
			}
		}
	}

	rows, err := q.ListRosterGames(ctx, db.ListRosterGamesParams{
		LeagueID: league.ID, FranchiseID: franchiseID, FromDay: sportsday.Date(first), ToDay: sportsday.Date(last),
	})
	if err != nil {
		return View{}, err
	}
	for _, r := range rows {
		if n := len(view.Players); n == 0 || view.Players[n-1].PlayerID != r.PlayerID {
			view.Players = append(view.Players, Player{
				PlayerID: r.PlayerID, FullName: r.FullName, Positions: r.Positions, HeadshotURL: r.HeadshotUrl,
				StarterEligible: rules.CanStart(league.Competition, r.Conference),
				TeamAbbrev:      r.TeamAbbrev, Slot: starting[r.PlayerID], CountsFrom: countsFrom[r.PlayerID], Games: []Game{},
			})
		}
		if !r.GameID.Valid {
			continue
		}
		p := &view.Players[len(view.Players)-1]
		p.Games = append(p.Games, Game{
			Day: r.GameDay.Time.Format(time.DateOnly), StartsAt: r.StartsAt.Time, Status: r.GameStatus,
			Opponent: r.Opponent, AtHome: r.AtHome, Points: r.Points, Counts: true,
		})
	}
	for i := range view.Players {
		p := &view.Players[i]
		if !p.StarterEligible {
			p.EligibilityNote = "Outside this league’s starting conferences"
			p.Slot = ""
		}
		settle(p, rules.Lineup, now)
	}
	return view, nil
}

// gamesCounted is how many games a week count in a slot; zero means all.
func gamesCounted(rules settings.Lineup, slot string) int {
	for _, s := range rules.Slots {
		if s.Name == slot {
			return s.GamesPerWeek
		}
	}
	return 0
}

// settle works out which of a player's games count, his points, and
// whether he can still be moved. Under the game-start rule:
//   - a starter whose every game counts locks when his first game begins;
//   - a starter in a slot that counts only some games locks when the first
//     game that counts begins, so an earlier game he sat out does not hold him;
//   - a bench player who could fill such a slot stays free while he has a
//     game left to count.
func settle(p *Player, rules settings.Lineup, now time.Time) {
	limit := gamesCounted(rules, p.Slot)
	counted := 0
	for i := range p.Games {
		g := &p.Games[i]
		if limit > 0 {
			g.Counts = g.Day >= p.CountsFrom && counted < limit
			if g.Counts {
				counted++
			}
		}
		if g.Counts {
			p.Points += g.Points
		}
	}
	if rules.Lock != settings.LockGameStart {
		return
	}
	started := func(g Game) bool { return !g.StartsAt.After(now) }
	switch {
	case limit > 0:
		p.Locked = slices.ContainsFunc(p.Games, func(g Game) bool { return g.Counts && started(g) })
	case p.Slot == "" && slices.ContainsFunc(rules.Slots, func(s settings.Slot) bool { return s.GamesPerWeek > 0 && fits(p.Positions, s) }):
		p.Locked = len(p.Games) > 0 && !slices.ContainsFunc(p.Games, func(g Game) bool { return !started(g) })
	default:
		p.Locked = slices.ContainsFunc(p.Games, started)
	}
}

// pick decides the day a starter's counted games begin in a slot that
// counts only some. Keeping a player where he was keeps his pick; a new
// pick must be for a game that has not started.
func pick(p Player, e Entry, first, last string, now time.Time, force bool) (string, error) {
	if p.Slot == e.Slot && (e.CountsFrom == "" || e.CountsFrom == p.CountsFrom) {
		return p.CountsFrom, nil
	}
	if p.Locked && !force {
		return "", problem.New("%s is locked: his game has started.", p.FullName)
	}
	day := e.CountsFrom
	if day == "" {
		// Nothing chosen: his next game, so one already played is not counted after the fact.
		if !slices.ContainsFunc(p.Games, func(g Game) bool { return !g.StartsAt.After(now) }) {
			return "", nil
		}
		i := slices.IndexFunc(p.Games, func(g Game) bool { return g.StartsAt.After(now) })
		if i < 0 {
			return "", problem.New("%s has no games left this week.", p.FullName)
		}
		return p.Games[i].Day, nil
	}
	if day < first || day > last {
		return "", problem.New("%s's game must be in the week of this lineup.", p.FullName)
	}
	i := slices.IndexFunc(p.Games, func(g Game) bool { return g.Day >= day })
	if i < 0 {
		return "", problem.New("%s has no game on or after %s this week.", p.FullName, day)
	}
	if !p.Games[i].StartsAt.After(now) && !force {
		return "", problem.New("%s's game on %s has already started.", p.FullName, p.Games[i].Day)
	}
	return p.Games[i].Day, nil
}

// Set replaces the lineup for a day (or the week containing it). Every
// starter must be on the main roster and fit his slot, and a player who has
// locked cannot be moved in or out. force is the commissioner's override of
// the locks.
func (s *Service) Set(ctx context.Context, league db.League, franchiseID pgtype.UUID, day time.Time, entries []Entry, force bool) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		// Serialize with roster moves for this franchise.
		if err := q.LockFranchiseRoster(ctx, db.LockFranchiseRosterParams{LeagueID: league.ID.String(), FranchiseID: franchiseID.String()}); err != nil {
			return err
		}
		current, err := load(ctx, q, league, franchiseID, day)
		if err != nil {
			return err
		}
		if current.Locked != "" && !force {
			return problem.Error(current.Locked)
		}

		wanted := map[pgtype.UUID]string{}
		filled := map[string]int{}
		counts := map[pgtype.UUID]pgtype.Date{}
		now := time.Now()
		for _, e := range entries {
			i := slices.IndexFunc(current.Players, func(p Player) bool { return p.PlayerID == e.PlayerID })
			if i < 0 {
				return problem.New("Only players on the main roster can start.")
			}
			player := current.Players[i]
			j := slices.IndexFunc(current.Slots, func(slot settings.Slot) bool { return slot.Name == e.Slot })
			if j < 0 {
				return problem.New("This league has no %q slot.", e.Slot)
			}
			slot := current.Slots[j]
			switch {
			case wanted[e.PlayerID] != "":
				return problem.New("%s is in the lineup twice.", player.FullName)
			case !player.StarterEligible:
				return problem.New("%s is outside this league’s starting conferences.", player.FullName)
			case !fits(player.Positions, slot):
				return problem.New("%s cannot play %s.", player.FullName, slot.Name)
			case filled[slot.Name] == slot.Count:
				return problem.New("The %s slot only holds %d.", slot.Name, slot.Count)
			}
			wanted[e.PlayerID] = e.Slot
			filled[slot.Name]++
			if slot.GamesPerWeek > 0 {
				day, err := pick(player, e, current.Day, current.LastDay, now, force)
				if err != nil {
					return err
				}
				if day != "" {
					from, _ := sportsday.Parse(day)
					counts[e.PlayerID] = sportsday.Date(from)
				}
			}
		}
		if !force {
			for _, p := range current.Players {
				if p.Locked && wanted[p.PlayerID] != p.Slot {
					return problem.New("%s is locked: his game has started.", p.FullName)
				}
			}
		}

		// Replace whatever takes effect within the span, including a
		// mid-week lineup left behind when a player was benched.
		first, _ := sportsday.Parse(current.Day)
		last, _ := sportsday.Parse(current.LastDay)
		key := db.DeleteLineupsParams{LeagueID: league.ID, FranchiseID: franchiseID, FromDay: sportsday.Date(first), ToDay: sportsday.Date(last)}
		if err := q.DeleteLineups(ctx, key); err != nil {
			return err
		}
		if err := q.InsertLineup(ctx, db.InsertLineupParams{LeagueID: league.ID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first)}); err != nil {
			return err
		}
		index := map[string]int32{}
		for _, e := range entries {
			if err := q.InsertLineupEntry(ctx, db.InsertLineupEntryParams{
				LeagueID: league.ID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first),
				Slot: e.Slot, SlotIndex: index[e.Slot], PlayerID: e.PlayerID, CountsFrom: counts[e.PlayerID],
			}); err != nil {
				return err
			}
			index[e.Slot]++
		}
		return nil
	})
}

// fits reports whether a player with these positions may fill the slot.
func fits(positions []string, slot settings.Slot) bool {
	if slices.Contains(slot.Positions, settings.AnyPosition) {
		return true
	}
	return slices.ContainsFunc(positions, func(p string) bool { return slices.Contains(slot.Positions, p) })
}

// Bench takes a player out of a franchise's lineups from now on, in the
// caller's transaction. It is called whenever he leaves the main roster, so
// he stops scoring for a franchise that no longer has him. What he has
// already scored stands: if his game today has begun, the change takes
// effect tomorrow.
func Bench(ctx context.Context, q *db.Queries, leagueID, franchiseID, playerID pgtype.UUID) error {
	day := sportsday.Today()
	started, err := q.PlayerHasStartedGame(ctx, db.PlayerHasStartedGameParams{PlayerID: playerID, Day: sportsday.Date(day)})
	if err != nil {
		return err
	}
	if started {
		day = day.AddDate(0, 0, 1)
	}

	inForce, err := q.LineupInForce(ctx, db.LineupInForceParams{LeagueID: leagueID, FranchiseID: franchiseID, Day: sportsday.Date(day)})
	if err != nil || !inForce.Valid {
		return err // no lineup has ever been set
	}
	// Start a new lineup on that day, the same as the one in force, so the
	// days before it keep their starters.
	if !inForce.Time.Equal(day) {
		if err := q.InsertLineup(ctx, db.InsertLineupParams{LeagueID: leagueID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(day)}); err != nil {
			return err
		}
		if err := q.CopyLineupEntries(ctx, db.CopyLineupEntriesParams{
			LeagueID: leagueID, FranchiseID: franchiseID, FromDay: inForce, ToDay: sportsday.Date(day),
		}); err != nil {
			return err
		}
	}
	return q.RemoveFromLineups(ctx, db.RemoveFromLineupsParams{
		LeagueID: leagueID, FranchiseID: franchiseID, PlayerID: playerID, FromDay: sportsday.Date(day),
	})
}
