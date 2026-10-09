// Package mlbam reads teams and 40-man rosters from MLB's public stats API.
package mlbam

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"crossover/internal/ingest"
)

const (
	defaultBase = "https://statsapi.mlb.com/api/v1"
	provider    = "mlbam"
	headshotURL = "https://img.mlbstatic.com/mlb-photos/image/upload/w_213,q_auto:best/v1/people/%d/headshot/67/current"
)

type Source struct {
	Base   string // overridable in tests
	client *ingest.Client

	mu          sync.Mutex
	roles       map[int]string // pitcher id -> "SP" or "RP"
	rolesRead   time.Time
	rolesSeason int // overridable in tests; 0 means this year
}

func New(client *ingest.Client) *Source {
	return &Source{Base: defaultBase, client: client}
}

func (s *Source) Teams(ctx context.Context) ([]ingest.Team, error) {
	var res struct {
		Teams []struct {
			ID           int    `json:"id"`
			Abbreviation string `json:"abbreviation"`
			Name         string `json:"name"`
		} `json:"teams"`
	}
	if err := s.client.GetJSON(ctx, s.Base+"/teams?sportId=1", &res); err != nil {
		return nil, err
	}

	var teams []ingest.Team
	for _, t := range res.Teams {
		teams = append(teams, ingest.Team{
			ProviderID: strconv.Itoa(t.ID),
			Abbrev:     t.Abbreviation,
			Name:       t.Name,
			LogoURL:    fmt.Sprintf("https://www.mlbstatic.com/team-logos/%d.svg", t.ID),
		})
	}
	return teams, nil
}

// Roster is the source of truth for team membership: the player detail
// endpoint leaves currentTeam empty for many active players.
func (s *Source) Roster(ctx context.Context, team ingest.Team) ([]ingest.Player, error) {
	var res struct {
		Roster []struct {
			Person struct {
				ID        int    `json:"id"`
				FullName  string `json:"fullName"`
				BirthDate string `json:"birthDate"`
			} `json:"person"`
			Position struct {
				Abbreviation string `json:"abbreviation"`
			} `json:"position"`
		} `json:"roster"`
	}
	url := fmt.Sprintf("%s/teams/%s/roster?rosterType=40Man&hydrate=person", s.Base, team.ProviderID)
	if err := s.client.GetJSON(ctx, url, &res); err != nil {
		return nil, err
	}

	var players []ingest.Player
	for _, r := range res.Roster {
		p := ingest.Player{
			Provider:    provider,
			ProviderID:  strconv.Itoa(r.Person.ID),
			FullName:    r.Person.FullName,
			BirthDate:   ingest.ParseDate(r.Person.BirthDate),
			HeadshotURL: fmt.Sprintf(headshotURL, r.Person.ID),
		}
		if r.Position.Abbreviation != "" {
			p.Positions = []string{r.Position.Abbreviation}
		}
		if r.Position.Abbreviation == "P" {
			role, err := s.pitcherRole(ctx, r.Person.ID)
			if err != nil {
				// Carrying on would turn every starter and reliever back into a plain pitcher.
				return nil, fmt.Errorf("pitcher roles: %w", err)
			}
			if role != "" {
				p.Positions = []string{role}
			}
		}
		players = append(players, p)
	}
	return players, nil
}

// pitcherRole says whether a pitcher starts or relieves. The feed calls
// them all "P", so it is read from how they have been used: a pitcher who
// started at least half his games this season and last is a starter. One
// who has not pitched in either stays a plain "P".
func (s *Source) pitcherRole(ctx context.Context, id int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.roles == nil || time.Since(s.rolesRead) > 12*time.Hour {
		season := s.rolesSeason
		if season == 0 {
			season = time.Now().Year()
		}
		type use struct{ games, starts float64 }
		used := map[int]use{}
		for _, year := range []int{season, season - 1} {
			var res struct {
				Stats []struct {
					Splits []struct {
						Player struct {
							ID int `json:"id"`
						} `json:"player"`
						Stat struct {
							Games  float64 `json:"gamesPlayed"`
							Starts float64 `json:"gamesStarted"`
						} `json:"stat"`
					} `json:"splits"`
				} `json:"stats"`
			}
			url := fmt.Sprintf("%s/stats?stats=season&group=pitching&season=%d&sportId=1&gameType=R&playerPool=all&limit=5000", s.Base, year)
			if err := s.client.GetTransient(ctx, url, &res); err != nil {
				return "", err
			}
			for _, stats := range res.Stats {
				for _, split := range stats.Splits {
					u := used[split.Player.ID]
					used[split.Player.ID] = use{u.games + split.Stat.Games, u.starts + split.Stat.Starts}
				}
			}
		}
		s.roles = map[int]string{}
		for player, u := range used {
			switch {
			case u.games == 0:
			case u.starts*2 >= u.games:
				s.roles[player] = "SP"
			default:
				s.roles[player] = "RP"
			}
		}
		s.rolesRead = time.Now()
	}
	return s.roles[id], nil
}

// draftYears is how many recent draft classes count as prospects.
const draftYears = 3

// minorLeagues are the levels below the majors, highest first, by the
// feed's sport ids: Triple-A, Double-A, High-A, Single-A and rookie ball.
const minorLeagues = "11,12,13,14,16"

// Prospects lists everyone who might reach the majors: recent draft picks,
// then every minor leaguer who has not played in the majors, which adds
// international signings and older draft classes. Both carry the ids they
// will have in the majors, so no merge is needed when they arrive. A player
// in both lists is described by where he plays now.
func (s *Source) Prospects(ctx context.Context) ([]ingest.Player, error) {
	players, err := s.draftPicks(ctx)
	if err != nil {
		return nil, err
	}
	minors, err := s.minorLeaguers(ctx)
	return append(players, minors...), err
}

func (s *Source) minorLeaguers(ctx context.Context) ([]ingest.Player, error) {
	season := time.Now().Year()
	if time.Now().Month() < time.April { // before opening day the lists are last year's
		season--
	}

	// Each club is described by its level and the major league club above it.
	var clubs struct {
		Teams []struct {
			ID            int    `json:"id"`
			ParentOrgName string `json:"parentOrgName"`
			Sport         struct {
				Name string `json:"name"`
			} `json:"sport"`
		} `json:"teams"`
	}
	if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/teams?sportIds=%s&season=%d", s.Base, minorLeagues, season), &clubs); err != nil {
		return nil, err
	}
	notes := map[int]string{}
	for _, t := range clubs.Teams {
		notes[t.ID] = t.Sport.Name
		if t.ParentOrgName != "" {
			notes[t.ID] += ", " + t.ParentOrgName
		}
	}

	var players []ingest.Player
	seen := map[int]bool{} // a promoted player is listed at each level he played
	for _, level := range strings.Split(minorLeagues, ",") {
		var res struct {
			People []struct {
				ID              int    `json:"id"`
				FullName        string `json:"fullName"`
				BirthDate       string `json:"birthDate"`
				MLBDebutDate    string `json:"mlbDebutDate"`
				PrimaryPosition struct {
					Abbreviation string `json:"abbreviation"`
				} `json:"primaryPosition"`
				CurrentTeam struct {
					ID int `json:"id"`
				} `json:"currentTeam"`
			} `json:"people"`
		}
		if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/sports/%s/players?season=%d", s.Base, level, season), &res); err != nil {
			return nil, err
		}
		for _, person := range res.People {
			if person.MLBDebutDate != "" || seen[person.ID] {
				continue
			}
			seen[person.ID] = true
			p := ingest.Player{
				Provider:    provider,
				ProviderID:  strconv.Itoa(person.ID),
				FullName:    person.FullName,
				BirthDate:   ingest.ParseDate(person.BirthDate),
				HeadshotURL: fmt.Sprintf(headshotURL, person.ID),
				Note:        notes[person.CurrentTeam.ID],
			}
			if pos := person.PrimaryPosition.Abbreviation; pos != "" {
				p.Positions = []string{pos}
			}
			players = append(players, p)
		}
	}
	return players, nil
}

func (s *Source) draftPicks(ctx context.Context) ([]ingest.Player, error) {
	// The draft is held in July.
	latest := time.Now().Year()
	if time.Now().Month() < time.August {
		latest--
	}

	var players []ingest.Player
	for year := latest; year > latest-draftYears; year-- {
		var res struct {
			Drafts struct {
				Rounds []struct {
					Picks []struct {
						PickRound string `json:"pickRound"`
						Person    struct {
							ID              int    `json:"id"`
							FullName        string `json:"fullName"`
							BirthDate       string `json:"birthDate"`
							PrimaryPosition struct {
								Abbreviation string `json:"abbreviation"`
							} `json:"primaryPosition"`
						} `json:"person"`
						School struct {
							Name string `json:"name"`
						} `json:"school"`
					} `json:"picks"`
				} `json:"rounds"`
			} `json:"drafts"`
		}
		if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/draft/%d", s.Base, year), &res); err != nil {
			return nil, err
		}
		for _, round := range res.Drafts.Rounds {
			for _, pick := range round.Picks {
				if pick.Person.ID == 0 { // a passed pick
					continue
				}
				p := ingest.Player{
					Provider:    provider,
					ProviderID:  strconv.Itoa(pick.Person.ID),
					FullName:    pick.Person.FullName,
					BirthDate:   ingest.ParseDate(pick.Person.BirthDate),
					HeadshotURL: fmt.Sprintf(headshotURL, pick.Person.ID),
					Note:        fmt.Sprintf("%d draft, round %s", year, pick.PickRound),
				}
				if pick.School.Name != "" {
					p.Note += ", " + pick.School.Name
				}
				if pos := pick.Person.PrimaryPosition.Abbreviation; pos != "" {
					p.Positions = []string{pos}
				}
				players = append(players, p)
			}
		}
	}
	return players, nil
}

// Games lists a day's games from the schedule.
func (s *Source) Games(ctx context.Context, day time.Time) ([]ingest.Game, error) {
	type club struct {
		Score int `json:"score"`
		Team  struct {
			ID int `json:"id"`
		} `json:"team"`
	}
	var res struct {
		Dates []struct {
			Games []struct {
				GamePk   int    `json:"gamePk"`
				GameDate string `json:"gameDate"`
				Status   struct {
					AbstractGameState string `json:"abstractGameState"` // Preview | Live | Final
				} `json:"status"`
				Teams struct {
					Home club `json:"home"`
					Away club `json:"away"`
				} `json:"teams"`
				Linescore struct {
					CurrentInningOrdinal string `json:"currentInningOrdinal"` // "5th"
					InningState          string `json:"inningState"`          // Top | Middle | Bottom | End
				} `json:"linescore"`
			} `json:"games"`
		} `json:"dates"`
	}
	url := fmt.Sprintf("%s/schedule?sportId=1&hydrate=linescore&date=%s", s.Base, day.Format(time.DateOnly))
	if err := s.client.GetTransient(ctx, url, &res); err != nil {
		return nil, err
	}

	status := map[string]string{"Live": ingest.GameLive, "Final": ingest.GameFinal}
	var games []ingest.Game
	for _, date := range res.Dates {
		for _, g := range date.Games {
			starts, err := time.Parse(time.RFC3339, g.GameDate)
			if err != nil {
				continue
			}
			game := ingest.Game{
				ProviderID: strconv.Itoa(g.GamePk), StartsAt: starts, Status: status[g.Status.AbstractGameState],
				HomeTeam: strconv.Itoa(g.Teams.Home.Team.ID), AwayTeam: strconv.Itoa(g.Teams.Away.Team.ID),
				HomeScore: g.Teams.Home.Score, AwayScore: g.Teams.Away.Score,
			}
			switch game.Status {
			case ingest.GameLive:
				game.Detail = strings.TrimSpace(g.Linescore.InningState + " " + g.Linescore.CurrentInningOrdinal)
			case ingest.GameFinal:
				game.Detail = "Final"
			default:
				game.Status = ingest.GameScheduled
			}
			games = append(games, game)
		}
	}
	return games, nil
}

// The feed's names for each stat, by canonical key.
var (
	battingStats = map[string]string{
		"bat_r": "runs", "bat_h": "hits", "bat_hr": "homeRuns", "bat_rbi": "rbi",
		"bat_sb": "stolenBases", "bat_bb": "baseOnBalls", "bat_so": "strikeOuts", "bat_tb": "totalBases",
	}
	pitchingStats = map[string]string{
		"pit_so": "strikeOuts", "pit_w": "wins", "pit_l": "losses", "pit_sv": "saves", "pit_hld": "holds",
		"pit_er": "earnedRuns", "pit_h": "hits", "pit_bb": "baseOnBalls",
	}
)

// BoxScore reads each player's batting and pitching lines. A player who
// did neither has empty lines and is left out.
func (s *Source) BoxScore(ctx context.Context, game ingest.Game) ([]ingest.StatLine, error) {
	type side struct {
		Players map[string]struct {
			Person struct {
				ID int `json:"id"`
			} `json:"person"`
			Stats struct {
				Batting  map[string]any `json:"batting"`
				Pitching map[string]any `json:"pitching"`
			} `json:"stats"`
		} `json:"players"`
	}
	var res struct {
		Teams struct {
			Home side `json:"home"`
			Away side `json:"away"`
		} `json:"teams"`
	}
	if err := s.client.GetTransient(ctx, fmt.Sprintf("%s/game/%s/boxscore", s.Base, game.ProviderID), &res); err != nil {
		return nil, err
	}

	var lines []ingest.StatLine
	for _, team := range []side{res.Teams.Home, res.Teams.Away} {
		for _, p := range team.Players {
			stats := map[string]float64{}
			if len(p.Stats.Batting) > 0 {
				for key, name := range battingStats {
					stats[key], _ = p.Stats.Batting[name].(float64)
				}
			}
			if innings, ok := p.Stats.Pitching["inningsPitched"].(string); ok {
				stats["pit_ip"] = inningsPitched(innings)
				for key, name := range pitchingStats {
					stats[key], _ = p.Stats.Pitching[name].(float64)
				}
			}
			if len(stats) > 0 {
				lines = append(lines, ingest.StatLine{Provider: provider, ProviderID: strconv.Itoa(p.Person.ID), Stats: stats})
			}
		}
	}
	return lines, nil
}

// LatestSeason is the year of the current MLB season, or of the last one
// before play starts in the spring.
func (s *Source) LatestSeason(now time.Time) int {
	if now.Month() >= time.April {
		return now.Year()
	}
	return now.Year() - 1
}

// SeasonStats reads every player's regular-season batting and pitching for
// one year. A two-way player's two lines are combined into one.
func (s *Source) SeasonStats(ctx context.Context, year int) ([]ingest.SeasonLine, error) {
	lines := map[int]*ingest.SeasonLine{}
	var order []int
	for _, group := range []string{"hitting", "pitching"} {
		var res struct {
			Stats []struct {
				Splits []struct {
					Player struct {
						ID int `json:"id"`
					} `json:"player"`
					Team struct {
						Name string `json:"name"`
					} `json:"team"`
					Stat map[string]any `json:"stat"`
				} `json:"splits"`
			} `json:"stats"`
		}
		url := fmt.Sprintf("%s/stats?stats=season&group=%s&season=%d&sportId=1&gameType=R&playerPool=all&limit=5000",
			s.Base, group, year)
		if err := s.client.GetTransient(ctx, url, &res); err != nil {
			return nil, err
		}
		for _, stats := range res.Stats {
			for _, split := range stats.Splits {
				line := lines[split.Player.ID]
				if line == nil {
					line = &ingest.SeasonLine{Provider: provider, ProviderID: strconv.Itoa(split.Player.ID), Season: ingest.Season{
						Year: year, Label: strconv.Itoa(year), Team: split.Team.Name, Stats: map[string]float64{},
					}}
					lines[split.Player.ID] = line
					order = append(order, split.Player.ID)
				}
				games, _ := split.Stat["gamesPlayed"].(float64)
				line.Games = max(line.Games, int(games))

				names := battingStats
				if group == "pitching" {
					names = pitchingStats
					innings, _ := split.Stat["inningsPitched"].(string)
					line.Stats["pit_ip"] = inningsPitched(innings)
				}
				for stat, name := range names {
					line.Stats[stat], _ = split.Stat[name].(float64)
				}
			}
		}
	}

	seasons := make([]ingest.SeasonLine, len(order))
	for i, id := range order {
		seasons[i] = *lines[id]
	}
	return seasons, nil
}

// inningsPitched converts baseball's notation, where the digit after the
// point counts outs, to a number: "6.1" is six and a third innings.
func inningsPitched(notation string) float64 {
	whole, outs, _ := strings.Cut(notation, ".")
	innings, _ := strconv.ParseFloat(whole, 64)
	thirds, _ := strconv.ParseFloat(outs, 64)
	return innings + thirds/3
}
