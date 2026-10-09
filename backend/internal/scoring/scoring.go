// Package scoring turns stat lines into standings and titles.
//
// Points are never stored. They are computed from the stat lines of whoever
// was starting each day and the league's scoring rules, so changing a rule
// changes every total at once.
package scoring

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
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

// Standings is a league's table for one season, best first.
type Standings struct {
	Season *db.Season `json:"season"` // nil until the commissioner starts one
	Format string     `json:"format"` // settings.FormatTotalPoints | settings.FormatHeadToHead
	Rows   []Row      `json:"rows"`
}

type Row struct {
	FranchiseID pgtype.UUID    `json:"franchise_id"`
	Points      float64        `json:"points"`
	Players     []PlayerPoints `json:"players"` // who scored them, best first
	// A head-to-head league's record over its finished regular-season matchups.
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Ties   int `json:"ties"`
}

type PlayerPoints struct {
	PlayerID    pgtype.UUID `json:"player_id"`
	FullName    string      `json:"full_name"`
	HeadshotURL string      `json:"headshot_url"`
	Games       int64       `json:"games"`
	Points      float64     `json:"points"`
}

// Standings returns the table for a league's latest season.
func (s *Service) Standings(ctx context.Context, league db.League) (Standings, error) {
	q := db.New(s.pool)
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return Standings{}, err
	}
	standings := Standings{Format: rules.Format.Type}

	season, err := q.LatestSeason(ctx, league.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		standings.Rows, err = table(ctx, q, league, nil)
	case err == nil:
		standings.Season = &season
		standings.Rows, err = table(ctx, q, league, &season)
	}
	return standings, err
}

// scores returns what each franchise's starters scored over a span of days,
// by franchise. Days that have not happened yet score nothing.
func scores(ctx context.Context, q *db.Queries, league db.League, from, to time.Time) (map[pgtype.UUID]Row, error) {
	if today := sportsday.Today(); today.Before(to) {
		to = today
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return nil, err
	}
	points, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{
		LeagueID: league.ID, FromDay: sportsday.Date(from), ToDay: sportsday.Date(to),
		WeekStart: sportsday.Date(sportsday.WeekStart(from, rules.Lineup.WeekStart)),
	})
	if err != nil {
		return nil, err
	}
	byFranchise := map[pgtype.UUID]Row{}
	for _, p := range points {
		row := byFranchise[p.FranchiseID]
		row.FranchiseID = p.FranchiseID
		row.Points += p.Points
		row.Players = append(row.Players, PlayerPoints{
			PlayerID: p.PlayerID, FullName: p.FullName, HeadshotURL: p.HeadshotUrl, Games: p.Games, Points: p.Points,
		})
		byFranchise[p.FranchiseID] = row
	}
	return byFranchise, nil
}

// table ranks every franchise for a season: by points, or in a
// head-to-head league by record and then points. Without a season everyone
// has zero.
func table(ctx context.Context, q *db.Queries, league db.League, season *db.Season) ([]Row, error) {
	franchises, err := q.ListFranchises(ctx, league.DynastyID)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, len(franchises))
	names := map[pgtype.UUID]string{}
	for i, f := range franchises {
		rows[i] = Row{FranchiseID: f.ID, Players: []PlayerPoints{}}
		names[f.ID] = f.Name
	}
	row := func(id pgtype.UUID) *Row {
		i := slices.IndexFunc(rows, func(r Row) bool { return r.FranchiseID == id })
		if i < 0 {
			return &Row{} // a franchise that has since left: its results are dropped
		}
		return &rows[i]
	}

	if season != nil {
		scored, err := scores(ctx, q, league, season.StartsOn.Time, season.EndsOn.Time)
		if err != nil {
			return nil, err
		}
		for id, s := range scored {
			r := row(id)
			r.Points, r.Players = s.Points, s.Players
		}
		results, err := regularSeasonResults(ctx, q, league, *season)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			switch home, away := row(result.Home), row(result.Away); {
			case result.HomePoints > result.AwayPoints:
				home.Wins++
				away.Losses++
			case result.HomePoints < result.AwayPoints:
				away.Wins++
				home.Losses++
			default:
				home.Ties++
				away.Ties++
			}
		}
	}

	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(share(b), share(a)),
			cmp.Compare(b.Points, a.Points),
			cmp.Compare(names[a.FranchiseID], names[b.FranchiseID]),
		)
	})
	return rows, nil
}

// share is the fraction of its matchups a franchise has won, a tie counting
// half. Ranking by it keeps records with a different number of matchups
// (after a bye) comparable.
func share(r Row) float64 {
	played := r.Wins + r.Losses + r.Ties
	if played == 0 {
		return 0
	}
	return (float64(r.Wins) + float64(r.Ties)/2) / float64(played)
}

// ---- seasons ----

// NewSeason is what the commissioner chooses when starting a season.
type NewSeason struct {
	LeagueID pgtype.UUID `json:"league_id"`
	Year     int         `json:"year"`
	StartsOn string      `json:"starts_on"` // YYYY-MM-DD
	EndsOn   string      `json:"ends_on"`
}

// CreateSeason starts a league's season. A league has one season at a
// time. A head-to-head league gets its schedule straight away.
func (s *Service) CreateSeason(ctx context.Context, in NewSeason) (db.Season, error) {
	starts, ends, err := dates(in.StartsOn, in.EndsOn)
	if err != nil {
		return db.Season{}, err
	}
	var season db.Season
	err = db.InTx(ctx, s.pool, func(q *db.Queries) error {
		league, err := q.GetLeague(ctx, in.LeagueID)
		if err != nil {
			return err
		}
		if latest, err := q.LatestSeason(ctx, in.LeagueID); err == nil {
			switch {
			case latest.Status == "active":
				return problem.New("Close the %d season before starting another.", latest.Year)
			case int(latest.Year) >= in.Year:
				return problem.New("This league already has a %d season.", latest.Year)
			}
		}
		season, err = q.CreateSeason(ctx, db.CreateSeasonParams{LeagueID: in.LeagueID, Year: int32(in.Year), StartsOn: starts, EndsOn: ends})
		if err != nil {
			return err
		}
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil || rules.Format.Type != settings.FormatHeadToHead {
			return err
		}
		return schedule(ctx, q, league, season)
	})
	return season, err
}

// SetSeasonDates changes when a season's games count.
func (s *Service) SetSeasonDates(ctx context.Context, seasonID pgtype.UUID, startsOn, endsOn string) error {
	starts, ends, err := dates(startsOn, endsOn)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if err := q.SetSeasonDates(ctx, db.SetSeasonDatesParams{ID: seasonID, StartsOn: starts, EndsOn: ends}); err != nil {
			return err
		}
		season, err := q.GetSeason(ctx, seasonID)
		if err != nil {
			return err
		}
		// The matchups are cut from the season's days, so they follow it.
		return reschedule(ctx, q, season.LeagueID)
	})
}

func dates(startsOn, endsOn string) (pgtype.Date, pgtype.Date, error) {
	starts, err1 := sportsday.Parse(startsOn)
	ends, err2 := sportsday.Parse(endsOn)
	if err1 != nil || err2 != nil {
		return pgtype.Date{}, pgtype.Date{}, problem.New("Season dates must look like 2026-10-20.")
	}
	if ends.Before(starts) {
		return pgtype.Date{}, pgtype.Date{}, problem.New("A season cannot end before it starts.")
	}
	return sportsday.Date(starts), sportsday.Date(ends), nil
}

// CloseSeason ends a season and names its champion: the winner of the
// playoff final in a head-to-head league, otherwise the franchise on top of
// the standings, unless the commissioner names another. A tie for first has
// to be settled by naming one.
func (s *Service) CloseSeason(ctx context.Context, seasonID, champion pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		season, err := q.GetSeason(ctx, seasonID)
		if err != nil {
			return err
		}
		if season.Status != "active" {
			return problem.New("That season is already closed.")
		}
		league, err := q.GetLeague(ctx, season.LeagueID)
		if err != nil {
			return err
		}
		rows, err := table(ctx, q, league, &season)
		if err != nil {
			return err
		}
		if !champion.Valid {
			// The winner of a finished playoff final, where there is one.
			if champion, err = playoffChampion(ctx, q, league, season); err != nil {
				return err
			}
		}
		if !champion.Valid {
			if len(rows) > 1 && rows[0].Points == rows[1].Points && rows[0].Wins == rows[1].Wins {
				return problem.New("First place is tied. Choose the champion.")
			}
			champion = rows[0].FranchiseID
		} else if !slices.ContainsFunc(rows, func(r Row) bool { return r.FranchiseID == champion }) {
			return problem.New("The champion must be a franchise in this league.")
		}
		return q.CompleteSeason(ctx, db.CompleteSeasonParams{ID: seasonID, ChampionFranchiseID: champion})
	})
}

// ReopenSeason undoes CloseSeason, for a season closed by mistake.
func (s *Service) ReopenSeason(ctx context.Context, seasonID pgtype.UUID) error {
	q := db.New(s.pool)
	season, err := q.GetSeason(ctx, seasonID)
	if err != nil {
		return err
	}
	if latest, err := q.LatestSeason(ctx, season.LeagueID); err != nil || latest.ID != season.ID {
		return problem.New("Only a league's most recent season can be reopened.")
	}
	return q.ReopenSeason(ctx, seasonID)
}

// ---- the overall title ----

// OverallYear is the cross-sport table for the seasons of one year: each
// franchise earns points for where it finished in each league.
type OverallYear struct {
	Year int `json:"year"`
	// Final is true once every league has finished its season for the year.
	Final bool         `json:"final"`
	Rows  []OverallRow `json:"rows"`
}

type OverallRow struct {
	FranchiseID pgtype.UUID    `json:"franchise_id"`
	Points      float64        `json:"points"`
	Finishes    map[string]int `json:"finishes"` // competition -> place, 1 being first
}

// Overall returns the cross-sport table for each year that has a season,
// newest first. It is empty when the dynasty has no overall title.
func (s *Service) Overall(ctx context.Context, dynasty db.Dynasty) ([]OverallYear, error) {
	rules, err := settings.Parse[settings.Dynasty](dynasty.Settings)
	if err != nil {
		return nil, err
	}
	years := []OverallYear{}
	if !rules.OverallTitle.Enabled {
		return years, nil
	}
	q := db.New(s.pool)
	leagues, err := q.ListLeagues(ctx, dynasty.ID)
	if err != nil {
		return nil, err
	}
	seasons, err := q.ListSeasons(ctx, dynasty.ID) // newest year first
	if err != nil {
		return nil, err
	}

	for _, season := range seasons {
		if len(years) == 0 || years[len(years)-1].Year != int(season.Year) {
			years = append(years, OverallYear{Year: int(season.Year), Final: true, Rows: []OverallRow{}})
		}
		year := &years[len(years)-1]
		league := leagues[slices.IndexFunc(leagues, func(l db.League) bool { return l.ID == season.LeagueID })]
		table, err := table(ctx, q, league, &season)
		if err != nil {
			return nil, err
		}
		year.Final = year.Final && season.Status == "complete"
		for place, row := range table {
			i := slices.IndexFunc(year.Rows, func(r OverallRow) bool { return r.FranchiseID == row.FranchiseID })
			if i < 0 {
				year.Rows = append(year.Rows, OverallRow{FranchiseID: row.FranchiseID, Finishes: map[string]int{}})
				i = len(year.Rows) - 1
			}
			year.Rows[i].Finishes[league.Competition] = place + 1
			if place < len(rules.OverallTitle.PointsByFinish) {
				year.Rows[i].Points += rules.OverallTitle.PointsByFinish[place]
			}
		}
	}
	for i := range years {
		// A year is only final when every league played it.
		years[i].Final = years[i].Final && len(years[i].Rows) > 0 && len(years[i].Rows[0].Finishes) == len(leagues)
		slices.SortStableFunc(years[i].Rows, func(a, b OverallRow) int { return cmp.Compare(b.Points, a.Points) })
	}
	return years, nil
}

// Window is the span of sports days a games sync covers by default: far
// enough back to catch stat corrections, far enough ahead to show a week's
// schedule.
func Window() (from, to time.Time) {
	today := sportsday.Today()
	return today.AddDate(0, 0, -3), today.AddDate(0, 0, 7)
}
