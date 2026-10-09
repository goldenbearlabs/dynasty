// Package nhle reads teams and rosters from the NHL's public web API.
package nhle

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"crossover/internal/ingest"
)

const (
	defaultBase        = "https://api-web.nhle.com/v1"
	defaultStatsBase   = "https://api.nhle.com/stats/rest/en"
	defaultRecordsBase = "https://records.nhl.com/site/api"
	provider           = "nhle"
)

type Source struct {
	Base        string // overridable in tests
	StatsBase   string // likewise, for the league-wide statistics
	RecordsBase string // and for the draft records
	client      *ingest.Client
}

func New(client *ingest.Client) *Source {
	return &Source{Base: defaultBase, StatsBase: defaultStatsBase, RecordsBase: defaultRecordsBase, client: client}
}

// localized is the NHL's shape for any display string.
type localized struct {
	Default string `json:"default"`
}

// Teams come from the standings, the one endpoint that lists every club.
// Rosters are addressed by abbreviation, so that is the provider id.
func (s *Source) Teams(ctx context.Context) ([]ingest.Team, error) {
	var res struct {
		Standings []struct {
			TeamAbbrev localized `json:"teamAbbrev"`
			TeamName   localized `json:"teamName"`
			TeamLogo   string    `json:"teamLogo"`
		} `json:"standings"`
	}
	if err := s.client.GetJSON(ctx, s.Base+"/standings/now", &res); err != nil {
		return nil, err
	}

	var teams []ingest.Team
	for _, t := range res.Standings {
		teams = append(teams, ingest.Team{
			ProviderID: t.TeamAbbrev.Default,
			Abbrev:     t.TeamAbbrev.Default,
			Name:       t.TeamName.Default,
			LogoURL:    t.TeamLogo,
		})
	}
	return teams, nil
}

type skater struct {
	ID           int       `json:"id"`
	FirstName    localized `json:"firstName"`
	LastName     localized `json:"lastName"`
	PositionCode string    `json:"positionCode"`
	BirthDate    string    `json:"birthDate"`
	Headshot     string    `json:"headshot"`
}

// Roster flattens the feed's three position groups. The "current" URL
// redirects to the season's roster, which the HTTP client follows.
func (s *Source) Roster(ctx context.Context, team ingest.Team) ([]ingest.Player, error) {
	var res struct {
		Forwards   []skater `json:"forwards"`
		Defensemen []skater `json:"defensemen"`
		Goalies    []skater `json:"goalies"`
	}
	if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/roster/%s/current", s.Base, team.ProviderID), &res); err != nil {
		return nil, err
	}

	var players []ingest.Player
	for _, group := range [][]skater{res.Forwards, res.Defensemen, res.Goalies} {
		for _, sk := range group {
			players = append(players, sk.player())
		}
	}
	return players, nil
}

// draftClasses is how many recent drafts' picks count as prospects. A club
// holds a pick's rights for two to four years.
const draftClasses = 5

// rankingProvider is the id space for players known only from the Central
// Scouting rankings, which carry no player id. See rankingKey.
const rankingProvider = "nhle_ranking"

// Prospects lists everyone who might reach the NHL, from three feeds:
// each club's own prospect list (which is patchy), every pick of the last
// few drafts, and the players ranked for the next draft. The first two
// carry the id a player keeps in the NHL. A ranked player has none until he
// is drafted, when his name and birth date tie the two together.
func (s *Source) Prospects(ctx context.Context) ([]ingest.Player, error) {
	players, err := s.clubProspects(ctx)
	if err != nil {
		return nil, err
	}
	// The draft is held in late June.
	latest := time.Now().Year()
	if time.Now().Month() < time.July {
		latest--
	}
	for year := latest; year > latest-draftClasses; year-- {
		picks, err := s.draftClass(ctx, year)
		if err != nil {
			return nil, err
		}
		players = append(players, picks...)
	}
	ranked, err := s.draftRankings(ctx, latest+1)
	if err != nil {
		return nil, err
	}
	return append(players, ranked...), nil
}

func (s *Source) clubProspects(ctx context.Context) ([]ingest.Player, error) {
	teams, err := s.Teams(ctx)
	if err != nil {
		return nil, err
	}

	var players []ingest.Player
	for _, team := range teams {
		var res struct {
			Forwards   []skater `json:"forwards"`
			Defensemen []skater `json:"defensemen"`
			Goalies    []skater `json:"goalies"`
		}
		if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/prospects/%s", s.Base, team.ProviderID), &res); err != nil {
			return nil, err
		}
		for _, group := range [][]skater{res.Forwards, res.Defensemen, res.Goalies} {
			for _, sk := range group {
				p := sk.player()
				p.Note = team.Abbrev + " prospect"
				players = append(players, p)
			}
		}
	}
	return players, nil
}

// draftClass lists the picks of one draft from the NHL's records, which,
// unlike the draft feed itself, give each pick his player id.
func (s *Source) draftClass(ctx context.Context, year int) ([]ingest.Player, error) {
	var res struct {
		Data []struct {
			PlayerID    int    `json:"playerId"`
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
			Position    string `json:"position"`
			BirthDate   string `json:"birthDate"`
			Round       int    `json:"roundNumber"`
			Overall     int    `json:"overallPickNumber"`
			Team        string `json:"triCode"`
			AmateurClub string `json:"amateurClubName"`
			League      string `json:"amateurLeague"`
		} `json:"data"`
	}
	address := fmt.Sprintf("%s/draft?cayenneExp=%s", s.RecordsBase, url.QueryEscape(fmt.Sprintf("draftYear=%d", year)))
	if err := s.client.GetJSON(ctx, address, &res); err != nil {
		return nil, err
	}

	var players []ingest.Player
	for _, pick := range res.Data {
		if pick.PlayerID == 0 {
			continue // a forfeited or voided pick
		}
		birth := ingest.ParseDate(pick.BirthDate)
		players = append(players, ingest.Player{
			Provider:   provider,
			ProviderID: strconv.Itoa(pick.PlayerID),
			FullName:   pick.FirstName + " " + pick.LastName,
			Positions:  []string{position(pick.Position)},
			BirthDate:  birth,
			Note: fmt.Sprintf("%d draft, round %d, #%d overall to %s. %s",
				year, pick.Round, pick.Overall, pick.Team, club(pick.AmateurClub, pick.League)),
			Aliases: []ingest.ExternalID{{Provider: rankingProvider, ProviderID: rankingKey(pick.FirstName, pick.LastName, birth)}},
		})
	}
	return players, nil
}

// draftRankings lists Central Scouting's ranked players for a draft: North
// American and international skaters and goalies. The list for a draft
// appears part-way through the season before it; until then there is none.
func (s *Source) draftRankings(ctx context.Context, year int) ([]ingest.Player, error) {
	categories := []string{"North American skaters", "International skaters", "North American goalies", "International goalies"}
	var players []ingest.Player
	for i, category := range categories {
		var res struct {
			Rankings []struct {
				FirstName    string `json:"firstName"`
				LastName     string `json:"lastName"`
				PositionCode string `json:"positionCode"`
				BirthDate    string `json:"birthDate"`
				Club         string `json:"lastAmateurClub"`
				League       string `json:"lastAmateurLeague"`
				MidtermRank  int    `json:"midtermRank"`
				FinalRank    int    `json:"finalRank"`
			} `json:"rankings"`
		}
		err := s.client.GetJSON(ctx, fmt.Sprintf("%s/draft/rankings/%d/%d", s.Base, year, i+1), &res)
		if ingest.NotFound(err) {
			return nil, nil // not published yet
		}
		if err != nil {
			return nil, err
		}
		for _, r := range res.Rankings {
			rank, when := r.FinalRank, "final"
			if rank == 0 {
				rank, when = r.MidtermRank, "midterm"
			}
			birth := ingest.ParseDate(r.BirthDate)
			players = append(players, ingest.Player{
				Provider:   rankingProvider,
				ProviderID: rankingKey(r.FirstName, r.LastName, birth),
				FullName:   r.FirstName + " " + r.LastName,
				Positions:  []string{position(r.PositionCode)},
				BirthDate:  birth,
				Note:       fmt.Sprintf("%d draft eligible, ranked #%d among %s (%s). %s", year, rank, category, when, club(r.Club, r.League)),
			})
		}
	}
	return players, nil
}

// rankingKey identifies a player who has no id yet by his name and birth
// date, which is how a ranked player is recognised when he is drafted.
func rankingKey(first, last string, birth time.Time) string {
	return strings.ToLower(first+" "+last) + "|" + birth.Format(time.DateOnly)
}

// position converts the draft's two-letter wings to the roster feed's codes.
func position(code string) string {
	switch code {
	case "LW":
		return "L"
	case "RW":
		return "R"
	}
	return code
}

func club(name, league string) string {
	if league == "" {
		return name
	}
	return name + " (" + league + ")"
}

func (sk skater) player() ingest.Player {
	birth := ingest.ParseDate(sk.BirthDate)
	return ingest.Player{
		Provider:    provider,
		ProviderID:  strconv.Itoa(sk.ID),
		FullName:    sk.FirstName.Default + " " + sk.LastName.Default,
		Positions:   []string{sk.PositionCode},
		BirthDate:   birth,
		HeadshotURL: sk.Headshot,
		// A ranked player who signs without being drafted is still the same person.
		Aliases: []ingest.ExternalID{{Provider: rankingProvider, ProviderID: rankingKey(sk.FirstName.Default, sk.LastName.Default, birth)}},
	}
}

// Games lists a day's games from the scores feed.
func (s *Source) Games(ctx context.Context, day time.Time) ([]ingest.Game, error) {
	type side struct {
		Abbrev string `json:"abbrev"`
		Score  int    `json:"score"`
	}
	var res struct {
		Games []struct {
			ID               int    `json:"id"`
			StartTimeUTC     string `json:"startTimeUTC"`
			GameState        string `json:"gameState"`
			HomeTeam         side   `json:"homeTeam"`
			AwayTeam         side   `json:"awayTeam"`
			Period           int    `json:"period"`
			PeriodDescriptor struct {
				PeriodType string `json:"periodType"` // REG | OT | SO
			} `json:"periodDescriptor"`
			Clock struct {
				TimeRemaining  string `json:"timeRemaining"`
				InIntermission bool   `json:"inIntermission"`
			} `json:"clock"`
		} `json:"games"`
	}
	if err := s.client.GetTransient(ctx, fmt.Sprintf("%s/score/%s", s.Base, day.Format(time.DateOnly)), &res); err != nil {
		return nil, err
	}

	status := map[string]string{
		"LIVE": ingest.GameLive, "CRIT": ingest.GameLive, // CRIT is the closing minutes
		"OFF": ingest.GameFinal, "FINAL": ingest.GameFinal,
	}
	var games []ingest.Game
	for _, g := range res.Games {
		starts, err := time.Parse(time.RFC3339, g.StartTimeUTC)
		if err != nil {
			continue
		}
		game := ingest.Game{
			ProviderID: strconv.Itoa(g.ID), StartsAt: starts, Status: status[g.GameState],
			HomeTeam: g.HomeTeam.Abbrev, AwayTeam: g.AwayTeam.Abbrev,
			HomeScore: g.HomeTeam.Score, AwayScore: g.AwayTeam.Score,
		}
		// The feed gives the period and clock as numbers; say them as a scoreboard would.
		period := map[string]string{"OT": "OT", "SO": "SO"}[g.PeriodDescriptor.PeriodType]
		if period == "" {
			period = ordinal(g.Period)
		}
		switch game.Status {
		case ingest.GameLive:
			game.Detail = g.Clock.TimeRemaining + " - " + period
			if g.Clock.InIntermission {
				game.Detail = "End " + period
			}
		case ingest.GameFinal:
			game.Detail = "Final"
			if g.PeriodDescriptor.PeriodType != "REG" && g.PeriodDescriptor.PeriodType != "" {
				game.Detail += "/" + period
			}
		default:
			game.Status = ingest.GameScheduled
		}
		games = append(games, game)
	}
	return games, nil
}

// BoxScore reads skaters and the goalies who played.
func (s *Source) BoxScore(ctx context.Context, game ingest.Game) ([]ingest.StatLine, error) {
	type skaterLine struct {
		PlayerID       int `json:"playerId"`
		Goals          int `json:"goals"`
		Assists        int `json:"assists"`
		Shots          int `json:"sog"`
		Hits           int `json:"hits"`
		Blocks         int `json:"blockedShots"`
		PowerPlayGoals int `json:"powerPlayGoals"`
		PIM            int `json:"pim"`
		PlusMinus      int `json:"plusMinus"`
	}
	type goalieLine struct {
		PlayerID     int    `json:"playerId"`
		TimeOnIce    string `json:"toi"`
		Saves        int    `json:"saves"`
		GoalsAgainst int    `json:"goalsAgainst"`
		Decision     string `json:"decision"` // "W" for the win
	}
	type side struct {
		Forwards []skaterLine `json:"forwards"`
		Defense  []skaterLine `json:"defense"`
		Goalies  []goalieLine `json:"goalies"`
	}
	var res struct {
		PlayerByGameStats struct {
			HomeTeam side `json:"homeTeam"`
			AwayTeam side `json:"awayTeam"`
		} `json:"playerByGameStats"`
	}
	if err := s.client.GetTransient(ctx, fmt.Sprintf("%s/gamecenter/%s/boxscore", s.Base, game.ProviderID), &res); err != nil {
		return nil, err
	}

	var lines []ingest.StatLine
	for _, team := range []side{res.PlayerByGameStats.HomeTeam, res.PlayerByGameStats.AwayTeam} {
		for _, group := range [][]skaterLine{team.Forwards, team.Defense} {
			for _, sk := range group {
				lines = append(lines, ingest.StatLine{Provider: provider, ProviderID: strconv.Itoa(sk.PlayerID), Stats: map[string]float64{
					"goals": float64(sk.Goals), "assists": float64(sk.Assists), "shots": float64(sk.Shots),
					"hits": float64(sk.Hits), "blocks": float64(sk.Blocks), "pp_goals": float64(sk.PowerPlayGoals),
					"pim": float64(sk.PIM), "plus_minus": float64(sk.PlusMinus),
				}})
			}
		}
		for _, g := range team.Goalies {
			if g.TimeOnIce == "" || g.TimeOnIce == "00:00" {
				continue // dressed but did not play
			}
			stats := map[string]float64{"saves": float64(g.Saves), "goals_against": float64(g.GoalsAgainst)}
			if g.Decision == "W" {
				stats["goalie_wins"] = 1
			}
			lines = append(lines, ingest.StatLine{Provider: provider, ProviderID: strconv.Itoa(g.PlayerID), Stats: stats})
		}
	}
	return lines, nil
}

// LatestSeason is the year the current NHL season ends in: seasons start in
// October and are named for both years.
func (s *Source) LatestSeason(now time.Time) int {
	if now.Month() >= time.October {
		return now.Year() + 1
	}
	return now.Year()
}

// report is one page-less listing from the NHL's statistics service.
func report[T any](ctx context.Context, s *Source, name string, year int) ([]T, error) {
	var res struct {
		Data []T `json:"data"`
	}
	// seasonId joins the two years: 20252026. gameTypeId 2 is the regular season.
	filter := fmt.Sprintf("seasonId=%d%d and gameTypeId=2", year-1, year)
	address := fmt.Sprintf("%s/%s?limit=-1&cayenneExp=%s", s.StatsBase, name, url.QueryEscape(filter))
	return res.Data, s.client.GetTransient(ctx, address, &res)
}

// SeasonStats reads every player's regular-season totals for the season
// ending in year. Skaters come from two reports, one with scoring and one
// with hits and blocked shots, joined by player; goalies from a third.
func (s *Source) SeasonStats(ctx context.Context, year int) ([]ingest.SeasonLine, error) {
	type skaterScoring struct {
		PlayerID    int     `json:"playerId"`
		Team        string  `json:"teamAbbrevs"`
		GamesPlayed int     `json:"gamesPlayed"`
		Goals       float64 `json:"goals"`
		Assists     float64 `json:"assists"`
		Shots       float64 `json:"shots"`
		PPGoals     float64 `json:"ppGoals"`
		PIM         float64 `json:"penaltyMinutes"`
		PlusMinus   float64 `json:"plusMinus"`
	}
	type skaterPhysical struct {
		PlayerID int     `json:"playerId"`
		Hits     float64 `json:"hits"`
		Blocks   float64 `json:"blockedShots"`
	}
	type goalie struct {
		PlayerID     int     `json:"playerId"`
		Team         string  `json:"teamAbbrevs"`
		GamesPlayed  int     `json:"gamesPlayed"`
		Wins         float64 `json:"wins"`
		Saves        float64 `json:"saves"`
		GoalsAgainst float64 `json:"goalsAgainst"`
	}
	scoring, err := report[skaterScoring](ctx, s, "skater/summary", year)
	if err != nil {
		return nil, err
	}
	physical, err := report[skaterPhysical](ctx, s, "skater/realtime", year)
	if err != nil {
		return nil, err
	}
	goalies, err := report[goalie](ctx, s, "goalie/summary", year)
	if err != nil {
		return nil, err
	}

	hitsAndBlocks := map[int]skaterPhysical{}
	for _, p := range physical {
		hitsAndBlocks[p.PlayerID] = p
	}
	line := func(playerID int, team string, games int, stats map[string]float64) ingest.SeasonLine {
		return ingest.SeasonLine{Provider: provider, ProviderID: strconv.Itoa(playerID), Season: ingest.Season{
			Year: year, Label: seasonLabel(year), Team: team, Games: games, Stats: stats,
		}}
	}
	var lines []ingest.SeasonLine
	for _, sk := range scoring {
		lines = append(lines, line(sk.PlayerID, sk.Team, sk.GamesPlayed, map[string]float64{
			"goals": sk.Goals, "assists": sk.Assists, "shots": sk.Shots, "pp_goals": sk.PPGoals,
			"pim": sk.PIM, "plus_minus": sk.PlusMinus,
			"hits": hitsAndBlocks[sk.PlayerID].Hits, "blocks": hitsAndBlocks[sk.PlayerID].Blocks,
		}))
	}
	for _, g := range goalies {
		lines = append(lines, line(g.PlayerID, g.Team, g.GamesPlayed, map[string]float64{
			"saves": g.Saves, "goals_against": g.GoalsAgainst, "goalie_wins": g.Wins,
		}))
	}
	return lines, nil
}

// seasonLabel names the season ending in year: "2025-26".
func seasonLabel(year int) string {
	return fmt.Sprintf("%d-%02d", year-1, year%100)
}

func (s *Source) IDSpace() string { return provider }

// Career reads a player's regular seasons outside the NHL from his
// profile: the junior, college, European and international hockey he came
// through, which is most of what there is to know about a prospect. His NHL
// seasons come from SeasonStats, which has more in it.
func (s *Source) Career(ctx context.Context, providerID string) ([]ingest.Season, error) {
	var res struct {
		Position     string `json:"position"`
		SeasonTotals []struct {
			Season       int       `json:"season"` // 20252026
			GameTypeID   int       `json:"gameTypeId"`
			LeagueAbbrev string    `json:"leagueAbbrev"`
			TeamName     localized `json:"teamName"`
			GamesPlayed  int       `json:"gamesPlayed"`
			Goals        float64   `json:"goals"`
			Assists      float64   `json:"assists"`
			Shots        float64   `json:"shots"`
			PIM          float64   `json:"pim"`
			PlusMinus    float64   `json:"plusMinus"`
			PPGoals      float64   `json:"powerPlayGoals"`
			Wins         float64   `json:"wins"`
			GoalsAgainst float64   `json:"goalsAgainst"`
			ShotsAgainst float64   `json:"shotsAgainst"`
		} `json:"seasonTotals"`
	}
	if err := s.client.GetTransient(ctx, fmt.Sprintf("%s/player/%s/landing", s.Base, providerID), &res); err != nil {
		return nil, err
	}

	const regularSeason = 2
	var seasons []ingest.Season
	for _, t := range res.SeasonTotals {
		if t.GameTypeID != regularSeason || t.LeagueAbbrev == "NHL" {
			continue
		}
		year := t.Season % 10000
		line := ingest.Season{Year: year, Label: seasonLabel(year), Team: t.TeamName.Default, League: t.LeagueAbbrev, Games: t.GamesPlayed}
		if res.Position == "G" {
			line.Stats = map[string]float64{
				"saves": max(t.ShotsAgainst-t.GoalsAgainst, 0), "goals_against": t.GoalsAgainst, "goalie_wins": t.Wins,
			}
		} else {
			line.Stats = map[string]float64{
				"goals": t.Goals, "assists": t.Assists, "shots": t.Shots,
				"pp_goals": t.PPGoals, "pim": t.PIM, "plus_minus": t.PlusMinus,
			}
		}
		seasons = append(seasons, line)
	}
	return seasons, nil
}

// ordinal names a period: 1st, 2nd, 3rd.
func ordinal(n int) string {
	suffix := map[int]string{1: "st", 2: "nd", 3: "rd"}[n]
	if suffix == "" {
		suffix = "th"
	}
	return strconv.Itoa(n) + suffix
}
