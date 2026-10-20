// Package lineup is who starts. A lineup takes effect on a sports day and
// stays in force until a later one replaces it, so nobody has to set one
// every day. Only starters score.
package lineup

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/projection"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

type Service struct {
	pool     *pgxpool.Pool
	registry competition.Registry // what each sport's stats move with, for projections
}

func NewService(pool *pgxpool.Pool, registry competition.Registry) *Service {
	return &Service{pool: pool, registry: registry}
}

// Entry puts one player in one starting slot.
type Entry struct {
	Slot     string      `json:"slot"`
	PlayerID pgtype.UUID `json:"player_id"`
	// CountsFrom picks the game that counts, in a slot that counts only
	// some games a week: the day of that game. Empty leaves it as it was,
	// or takes the player's next game.
	CountsFrom string `json:"counts_from"`
	// Auto, in such a slot, counts his last game of the week in which he
	// recorded a stat, in place of a game picked ahead.
	Auto bool `json:"auto"`
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
	// Starts is the week's pitcher starts against the league's cap; nil in
	// a league without one.
	Starts *Starts `json:"starts"`
	// Doubled is set in a matchup the commissioner has doubled: the slots
	// that count only some games count twice as many, and the pitcher
	// start cap doubles, all over the whole matchup.
	Doubled *Doubled `json:"doubled"`
}

// Doubled is the doubled matchup a lineup falls in.
type Doubled struct {
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
}

// Starts is how many pitcher starts a franchise has used in the week of a
// lineup, or in its doubled matchup. Those beyond the limit, in the order played, score nothing.
type Starts struct {
	Limit int     `json:"limit"`
	Made  []Start `json:"made"`
}

type Start struct {
	PlayerID pgtype.UUID `json:"player_id"`
	FullName string      `json:"full_name"`
	Day      string      `json:"day"`
	Innings  float64     `json:"innings"`
	Counts   bool        `json:"counts"`
}

type Player struct {
	Nickname        string      `json:"nickname"`
	StarterEligible bool        `json:"starter_eligible"`
	EligibilityNote string      `json:"eligibility_note"`
	PlayerID        pgtype.UUID `json:"player_id"`
	FullName        string      `json:"full_name"`
	Positions       []string    `json:"positions"`
	HeadshotURL     string      `json:"headshot_url"`
	TeamAbbrev      string      `json:"team_abbrev"`
	Injury          string      `json:"injury"`      // an injured-list designation, or empty
	Slot            string      `json:"slot"`        // empty on the bench
	CountsFrom      string      `json:"counts_from"` // the day his counted games begin, when his slot counts only some
	CountsLast      bool        `json:"counts_last"` // in such a slot, on auto: his last games with a stat count
	Locked          bool        `json:"locked"`
	// Points over the day or week, whether he started or not. For a starter
	// in a slot that counts only some games, only the games that count.
	Points float64 `json:"points"`
	Games  []Game  `json:"games"`
}

type Game struct {
	GameID   pgtype.UUID `json:"game_id"`
	Day      string      `json:"day"`
	StartsAt time.Time   `json:"starts_at"`
	Status   string      `json:"status"`
	Opponent string      `json:"opponent"`
	AtHome   bool        `json:"at_home"`
	Points   float64     `json:"points"`
	Counts   bool        `json:"counts"` // false for a game his slot leaves out
	// Starter is whether a feed has him starting in goal or on the mound:
	// confirmed, expected, other (a teammate at his position is), or empty.
	Starter string `json:"starter"`
	// Projected is his fantasy points in a game still to be played, from
	// his recent seasons and the betting line; zero once it starts.
	Projected float64 `json:"projected"`
	market    projection.Game
	// Stats is his stat line, by the competition's stat keys; empty until he plays.
	Stats json.RawMessage `json:"stats"`
}

// Get returns the lineup in force on a day.
func (s *Service) Get(ctx context.Context, league db.League, franchiseID pgtype.UUID, day time.Time) (View, error) {
	q := db.New(s.pool)
	view, err := load(ctx, q, league, franchiseID, day)
	if err != nil {
		return view, err
	}
	return view, s.project(ctx, q, league, &view)
}

// project fills in each game still to be played with the player's
// projected fantasy points under the league's scoring.
func (s *Service) project(ctx context.Context, q *db.Queries, league db.League, view *View) error {
	if len(view.Players) == 0 {
		return nil
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	c, _ := s.registry.Get(league.Competition)
	ids := make([]pgtype.UUID, len(view.Players))
	for i, p := range view.Players {
		ids[i] = p.PlayerID
	}
	rows, err := q.ListProjectionSeasons(ctx, db.ListProjectionSeasonsParams{PlayerIds: ids, Competition: league.Competition})
	if err != nil {
		return err
	}
	runs, err := q.ListSeasonLengths(ctx, league.Competition)
	if err != nil {
		return err
	}
	average, err := q.AverageImplied(ctx, league.Competition)
	if err != nil {
		return err
	}
	lengths := map[int]float64{}
	for _, r := range runs {
		lengths[int(r.Year)] = float64(r.Games)
	}
	seasons := map[pgtype.UUID][]projection.Season{}
	for _, r := range rows {
		season := projection.Season{Year: int(r.Year), Games: float64(r.Games)}
		if err := json.Unmarshal(r.Stats, &season.Stats); err != nil {
			return err
		}
		seasons[r.PlayerID] = append(seasons[r.PlayerID], season)
	}
	for i := range view.Players {
		p := &view.Players[i]
		perGame, plays := projection.Baseline(seasons[p.PlayerID], lengths)
		for j := range p.Games {
			if g := &p.Games[j]; g.Status == "scheduled" {
				g.market.Average = average
				g.Projected = projection.Points(perGame, plays, g.market, rules.Scoring, c.Follows)
			}
		}
	}
	return nil
}

// span is the run of days one lineup covers: the day itself, or its week.
func span(rules settings.Lineup, day time.Time) (first, last time.Time) {
	if rules.Period == settings.PeriodWeek {
		first = sportsday.WeekStart(day, rules.WeekStart)
		return first, first.AddDate(0, 0, 6)
	}
	return day, day
}

// doubledOn is the doubled matchup a day falls in, if any.
func doubledOn(ctx context.Context, q *db.Queries, leagueID pgtype.UUID, day time.Time) (db.Period, bool, error) {
	p, err := q.DoublePeriodOn(ctx, db.DoublePeriodOnParams{LeagueID: leagueID, Day: sportsday.Date(day)})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	return p, err == nil, err
}

func load(ctx context.Context, q *db.Queries, league db.League, franchiseID pgtype.UUID, day time.Time) (View, error) {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return View{}, err
	}
	first, last := span(rules.Lineup, day)
	double, doubled, err := doubledOn(ctx, q, league.ID, day)
	if err != nil {
		return View{}, err
	}
	mult := 1
	if doubled {
		mult = 2
		if rules.Lineup.Period == settings.PeriodWeek {
			// One lineup for the whole matchup.
			first, last = double.StartsOn.Time, double.EndsOn.Time
		}
	}
	view := View{
		Day: first.Format(time.DateOnly), LastDay: last.Format(time.DateOnly),
		Slots: rules.Lineup.Slots, Players: []Player{},
	}
	if doubled {
		view.Doubled = &Doubled{StartsOn: double.StartsOn.Time.Format(time.DateOnly), EndsOn: double.EndsOn.Time.Format(time.DateOnly)}
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
	countsLast := map[pgtype.UUID]bool{}
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
			countsLast[e.PlayerID] = e.CountsLast
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
				PlayerID: r.PlayerID, FullName: r.FullName, Nickname: r.Nickname, Positions: r.Positions, HeadshotURL: r.HeadshotUrl,
				StarterEligible: rules.CanStart(league.Competition, r.Conference),
				TeamAbbrev:      r.TeamAbbrev, Injury: r.InjuryDesignation, Slot: starting[r.PlayerID], CountsFrom: countsFrom[r.PlayerID], CountsLast: countsLast[r.PlayerID], Games: []Game{},
			})
		}
		if !r.GameID.Valid {
			continue
		}
		p := &view.Players[len(view.Players)-1]
		p.Games = append(p.Games, Game{
			GameID: r.GameID, Day: r.GameDay.Time.Format(time.DateOnly), StartsAt: r.StartsAt.Time, Status: r.GameStatus,
			Opponent: r.Opponent, AtHome: r.AtHome, Points: r.Points, Counts: true, Stats: r.Stats, Starter: r.Starter,
			market: projection.Game{
				TeamScore: r.TeamImplied, OpponentScore: r.OpponentImplied, Win: r.WinChance,
				Starter: r.Starter, Injured: r.InjuryDesignation != "",
			},
		})
	}
	for i := range view.Players {
		p := &view.Players[i]
		if !p.StarterEligible {
			p.EligibilityNote = "Outside this league’s starting conferences"
			p.Slot = ""
		}
		settle(p, rules.Lineup, now, mult)
	}
	if limit := rules.Lineup.PitcherStartsPerWeek; limit > 0 {
		from := sportsday.WeekStart(first, rules.Lineup.WeekStart)
		to := from.AddDate(0, 0, 6)
		if doubled {
			from, to, limit = double.StartsOn.Time, double.EndsOn.Time, limit*mult
		}
		made, err := q.ListWeekStarts(ctx, db.ListWeekStartsParams{
			LeagueID: league.ID, FranchiseID: franchiseID,
			WeekStart: sportsday.Date(from), WeekEnd: sportsday.Date(to),
		})
		if err != nil {
			return View{}, err
		}
		view.Starts = &Starts{Limit: limit, Made: []Start{}}
		for i, m := range made {
			view.Starts.Made = append(view.Starts.Made, Start{
				PlayerID: m.PlayerID, FullName: m.FullName, Day: m.Day.Time.Format(time.DateOnly), Innings: m.Innings, Counts: i < limit,
			})
		}
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
//
// mult is 2 in a doubled matchup, where such a slot counts twice the games.
func settle(p *Player, rules settings.Lineup, now time.Time, mult int) {
	limit := gamesCounted(rules, p.Slot) * mult
	counted := 0
	if limit > 0 && p.CountsLast {
		// On auto, his last games of the week in which he recorded a stat.
		for i := range p.Games {
			p.Games[i].Counts = false
		}
		for i := len(p.Games) - 1; i >= 0 && counted < limit; i-- {
			if g := &p.Games[i]; g.Day >= p.CountsFrom && recorded(g.Stats) {
				g.Counts = true
				counted++
			}
		}
	}
	for i := range p.Games {
		g := &p.Games[i]
		if limit > 0 && !p.CountsLast {
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
	case limit > 0 && p.CountsLast:
		// Any game of his may yet be the one that counts, so the first to begin holds him.
		p.Locked = slices.ContainsFunc(p.Games, func(g Game) bool { return g.Day >= p.CountsFrom && started(g) })
	case limit > 0:
		p.Locked = slices.ContainsFunc(p.Games, func(g Game) bool { return g.Counts && started(g) })
	case p.Slot == "" && slices.ContainsFunc(rules.Slots, func(s settings.Slot) bool { return s.GamesPerWeek > 0 && Fits(p.Positions, s) }):
		p.Locked = len(p.Games) > 0 && !slices.ContainsFunc(p.Games, func(g Game) bool { return !started(g) })
	default:
		p.Locked = slices.ContainsFunc(p.Games, started)
	}
}

// recorded reports whether a stat line has any stat in it: whether he
// played, as far as the box score says.
func recorded(stats json.RawMessage) bool {
	var line map[string]float64
	json.Unmarshal(stats, &line)
	for _, n := range line {
		if n != 0 {
			return true
		}
	}
	return false
}

// pick decides the day a starter's counted games begin in a slot that
// counts only some. Keeping a player where he was keeps his pick; a new
// pick must be for a game that has not started. On auto, the games he has
// already played this week count only if he was there for them: a player
// coming in counts from his next game.
func pick(p Player, e Entry, first, last string, now time.Time, force bool) (string, error) {
	if p.Slot == e.Slot && e.Auto == p.CountsLast && (e.Auto || e.CountsFrom == "" || e.CountsFrom == p.CountsFrom) {
		return p.CountsFrom, nil
	}
	if p.Locked && !force {
		return "", problem.New("%s is locked: his game has started.", p.FullName)
	}
	day := e.CountsFrom
	if e.Auto || day == "" {
		// Auto, or nothing chosen: from his next game, so one already played
		// is not counted after the fact.
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
		onAuto := map[pgtype.UUID]bool{}
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
			case !Fits(player.Positions, slot):
				return problem.New("%s cannot play %s.", player.FullName, slot.Name)
			case filled[slot.Name] == slot.Count:
				return problem.New("The %s slot only holds %d.", slot.Name, slot.Count)
			}
			wanted[e.PlayerID] = e.Slot
			filled[slot.Name]++
			if slot.GamesPerWeek > 0 {
				onAuto[e.PlayerID] = e.Auto
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
				Slot: e.Slot, SlotIndex: index[e.Slot], PlayerID: e.PlayerID, CountsFrom: counts[e.PlayerID], CountsLast: onAuto[e.PlayerID],
			}); err != nil {
				return err
			}
			index[e.Slot]++
		}
		return nil
	})
}

// AutoSet sets the best lineup it can for each day still to come in the
// week of day, or for the week itself in a weekly league. A player who has
// locked stays where he is. The other places go to as many players with a
// game still to play as can fill them, then to the rest; where more play
// than fit, the higher projections for those games start, then those with
// more fantasy points in the sport's last season. In a slot that counts only some games, every
// starter who is free to move is put on auto, so his last game of the week
// that he plays in counts.
func (s *Service) AutoSet(ctx context.Context, league db.League, franchiseID pgtype.UUID, day time.Time) error {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	ranked, err := db.New(s.pool).ListRankedPlayers(ctx, db.ListRankedPlayersParams{Competition: league.Competition, PageSize: 5000})
	if err != nil {
		return err
	}
	rank := map[pgtype.UUID]int{}
	for i, p := range ranked {
		rank[p.ID] = i + 1
	}
	from, last := span(rules.Lineup, day)
	if rules.Lineup.Period != settings.PeriodWeek {
		// Every day left in the week, but none already gone.
		from, last = day, sportsday.WeekStart(day, rules.Lineup.WeekStart).AddDate(0, 0, 6)
		if today := sportsday.Today(); from.Before(today) {
			from = today
		}
	}
	for d := from; !d.After(last); d = d.AddDate(0, 0, 1) {
		view, err := s.Get(ctx, league, franchiseID, d)
		if err != nil {
			return err
		}
		if view.Locked == "" {
			if err := s.Set(ctx, league, franchiseID, d, best(view, rank, time.Now()), false); err != nil {
				return err
			}
		}
		if rules.Lineup.Period == settings.PeriodWeek {
			break
		}
	}
	return nil
}

// best picks a lineup from a view: see AutoSet.
func best(view View, rank map[pgtype.UUID]int, now time.Time) []Entry {
	ahead := func(p Player) bool {
		return slices.ContainsFunc(p.Games, func(g Game) bool { return g.StartsAt.After(now) })
	}
	order := func(p Player) int {
		r, ok := rank[p.PlayerID]
		if !ok {
			r = len(rank) + 1
		}
		if !ahead(p) {
			r += len(rank) + 2 // a game to come first, then everyone else
		}
		return r
	}
	projected := func(p Player) (n float64) {
		for _, g := range p.Games {
			if g.StartsAt.After(now) {
				n += g.Projected
			}
		}
		return n
	}
	players := slices.SortedFunc(slices.Values(view.Players), func(a, b Player) int {
		if ahead(a) != ahead(b) {
			return order(a) - order(b)
		}
		return cmp.Or(cmp.Compare(projected(b), projected(a)), order(a)-order(b))
	})

	var entries []Entry
	taken := map[pgtype.UUID]bool{}
	filled := map[string]int{}
	keep := func(p Player) Entry {
		return Entry{Slot: p.Slot, PlayerID: p.PlayerID, CountsFrom: p.CountsFrom, Auto: p.CountsLast}
	}
	for _, p := range players {
		if p.Locked && p.Slot != "" {
			entries = append(entries, keep(p))
			taken[p.PlayerID] = true
			filled[p.Slot]++
		}
	}
	// The open seats, slot by slot.
	var seats []settings.Slot
	for _, slot := range view.Slots {
		for range slot.Count - filled[slot.Name] {
			seats = append(seats, slot)
		}
	}
	fits := func(p Player, slot settings.Slot) bool {
		switch {
		case p.Locked || !p.StarterEligible || !Fits(p.Positions, slot):
			return false
		case slot.GamesPerWeek > 0 && p.Slot != slot.Name:
			// Coming into a slot that counts some games: he needs one still to play.
			return ahead(p)
		}
		return true
	}
	// Seat the players best first. When the seats he fits are full, one
	// who fits elsewhere moves over for him, so a player who could fill
	// two slots never keeps the only one another fits; nobody seated is
	// ever unseated, so those with a game to come start ahead of the rest
	// and, among them, the higher projections.
	sitter := make([]int, len(seats)) // index into players, or -1
	for i := range sitter {
		sitter[i] = -1
	}
	var seat func(p int, tried []bool) bool
	seat = func(p int, tried []bool) bool {
		for s, slot := range seats {
			if tried[s] || !fits(players[p], slot) {
				continue
			}
			tried[s] = true
			if sitter[s] < 0 || seat(sitter[s], tried) {
				sitter[s] = p
				return true
			}
		}
		return false
	}
	for p := range players {
		if !taken[players[p].PlayerID] {
			seat(p, make([]bool, len(seats)))
		}
	}
	for s, p := range sitter {
		if p >= 0 {
			entries = append(entries, Entry{Slot: seats[s].Name, PlayerID: players[p].PlayerID, Auto: seats[s].GamesPerWeek > 0})
		}
	}
	return entries
}

// Fits reports whether a player with these positions may fill the slot.
func Fits(positions []string, slot settings.Slot) bool {
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
