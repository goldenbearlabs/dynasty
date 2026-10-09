// Package espn reads teams and rosters from ESPN's public site API.
// It serves every ESPN-backed competition; only the league path differs.
package espn

import (
	"context"
	"fmt"
	"html"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"crossover/internal/ingest"
)

const (
	defaultBase    = "https://site.api.espn.com/apis/site/v2/sports"
	defaultWebBase = "https://site.web.api.espn.com/apis/common/v3/sports"
)

// League is one ESPN league and how to read it.
type League struct {
	Path     string // e.g. "basketball/nba"
	Provider string // athlete id space, e.g. "espn_basketball"
	// Stats maps ESPN's box score stat names to the competition's canonical
	// keys. Where a name appears in more than one group it is prefixed with
	// the group: "passing.interceptions".
	Stats map[string]string
	// Scoreboard is extra query for the scoreboard, for leagues whose
	// default listing leaves games out.
	Scoreboard string
	// SeasonGroups are the groups of season stats to read. The feed repeats
	// some stats across groups (touchdowns appear under both "rushing" and
	// "scoring"), so only these are counted.
	SeasonGroups []string
	// SeasonStarts is the month the regular season begins, and
	// SeasonSpansYears whether it runs across the new year, which decides
	// how ESPN numbers it. See LatestSeason.
	SeasonStarts     time.Month
	SeasonSpansYears bool
	// Derive adds stats worked out from a player's line in one game, such
	// as a double-double. It may be nil.
	Derive func(stats map[string]float64)
	// Positions, when set, limits the league to players at these positions.
	Positions []string
	// Groups, when set, limits the league to the teams in these ESPN
	// groups: for college sports, conference ids.
	Groups []string
}

type Source struct {
	Base     string // overridable in tests
	WebBase  string // likewise, for the league-wide statistics
	CoreBase string // likewise, for group membership
	client   *ingest.Client
	League
}

// New returns a Source for one ESPN league. Leagues that share athlete ids
// (college and pro basketball) must share a provider name.
func New(client *ingest.Client, league League) *Source {
	return &Source{Base: defaultBase, WebBase: defaultWebBase, CoreBase: defaultCoreBase, client: client, League: league}
}

// Pool reports how the league has been narrowed, so that players stored
// before the narrowing can be removed.
func (s *Source) Pool() ingest.Pool {
	return ingest.Pool{Positions: s.Positions, ListedTeamsOnly: len(s.Groups) > 0}
}

func (s *Source) Teams(ctx context.Context) ([]ingest.Team, error) {
	var res struct {
		Sports []struct {
			Leagues []struct {
				Teams []struct {
					Team struct {
						ID           string `json:"id"`
						Abbreviation string `json:"abbreviation"`
						DisplayName  string `json:"displayName"`
						Logos        []struct {
							Href string `json:"href"`
						} `json:"logos"`
					} `json:"team"`
				} `json:"teams"`
			} `json:"leagues"`
		} `json:"sports"`
	}
	// limit=500 returns every college team in one page.
	if err := s.client.GetJSON(ctx, fmt.Sprintf("%s/%s/teams?limit=500", s.Base, s.Path), &res); err != nil {
		return nil, err
	}
	if len(res.Sports) == 0 || len(res.Sports[0].Leagues) == 0 {
		return nil, fmt.Errorf("espn %s: no league in teams response", s.Path)
	}

	members, err := s.groupMembers(ctx)
	if err != nil {
		return nil, err
	}
	var teams []ingest.Team
	for _, t := range res.Sports[0].Leagues[0].Teams {
		if members != nil && !members[t.Team.ID] {
			continue
		}
		team := ingest.Team{ProviderID: t.Team.ID, Abbrev: t.Team.Abbreviation, Name: t.Team.DisplayName}
		if len(t.Team.Logos) > 0 {
			team.LogoURL = t.Team.Logos[0].Href
		}
		teams = append(teams, team)
	}
	return teams, nil
}

// groupMembers returns the ids of the teams in the league's Groups, or nil
// when the league is not limited to any. Membership is read for the season
// about to start, since teams change conference over the summer, falling
// back to the one before until the new one is published. A group with no
// teams is an error: carrying on would drop a whole conference.
func (s *Source) groupMembers(ctx context.Context) (map[string]bool, error) {
	if len(s.Groups) == 0 {
		return nil, nil
	}
	sport, league, _ := strings.Cut(s.Path, "/")
	season := time.Now().Year()
	if time.Now().Month() >= time.July {
		season++
	}

	members := map[string]bool{}
	for _, group := range s.Groups {
		var res struct {
			Items []ref `json:"items"`
		}
		for _, year := range []int{season, season - 1} {
			url := fmt.Sprintf("%s/%s/leagues/%s/seasons/%d/types/2/groups/%s/teams?limit=200", s.CoreBase, sport, league, year, group)
			if err := s.client.GetJSON(ctx, url, &res); err != nil && !ingest.NotFound(err) {
				return nil, err
			}
			if len(res.Items) > 0 {
				break
			}
		}
		if len(res.Items) == 0 {
			return nil, fmt.Errorf("espn %s: group %s lists no teams", s.Path, group)
		}
		for _, team := range res.Items {
			members[team.id()] = true
		}
	}
	return members, nil
}

func (s *Source) Roster(ctx context.Context, team ingest.Team) ([]ingest.Player, error) {
	var res struct {
		Team struct {
			Athletes []struct {
				ID          string `json:"id"`
				FullName    string `json:"fullName"`
				DateOfBirth string `json:"dateOfBirth"`
				Position    struct {
					Abbreviation string `json:"abbreviation"`
				} `json:"position"`
				Headshot struct {
					Href string `json:"href"`
				} `json:"headshot"`
				Experience struct {
					Abbreviation string `json:"abbreviation"` // class year, college only
				} `json:"experience"`
			} `json:"athletes"`
		} `json:"team"`
	}
	url := fmt.Sprintf("%s/%s/teams/%s?enable=roster", s.Base, s.Path, team.ProviderID)
	if err := s.client.GetJSON(ctx, url, &res); err != nil {
		return nil, err
	}

	var players []ingest.Player
	for _, a := range res.Team.Athletes {
		name := strings.TrimSpace(a.FullName)
		if name == "" || name == "Team" { // ESPN lists a placeholder "Team" athlete on some rosters
			continue
		}
		p := ingest.Player{
			Provider:    s.Provider,
			ProviderID:  a.ID,
			FullName:    name,
			BirthDate:   ingest.ParseDate(a.DateOfBirth),
			Class:       a.Experience.Abbreviation,
			HeadshotURL: a.Headshot.Href,
		}
		if a.Position.Abbreviation != "" {
			p.Positions = []string{a.Position.Abbreviation}
		}
		if len(s.Positions) > 0 && !slices.Contains(s.Positions, a.Position.Abbreviation) {
			continue
		}
		players = append(players, p)
	}
	return players, nil
}

// Games lists a day's games from the scoreboard.
func (s *Source) Games(ctx context.Context, day time.Time) ([]ingest.Game, error) {
	var res struct {
		Events []struct {
			ID     string `json:"id"`
			Date   string `json:"date"`
			Status struct {
				Type struct {
					State       string `json:"state"`       // pre | in | post
					ShortDetail string `json:"shortDetail"` // "7:32 - 3rd", "Final/OT"
				} `json:"type"`
			} `json:"status"`
			Competitions []struct {
				Competitors []struct {
					HomeAway string `json:"homeAway"`
					Score    string `json:"score"`
					Team     struct {
						ID string `json:"id"`
					} `json:"team"`
				} `json:"competitors"`
			} `json:"competitions"`
		} `json:"events"`
	}
	url := fmt.Sprintf("%s/%s/scoreboard?dates=%s", s.Base, s.Path, day.Format("20060102"))
	if s.Scoreboard != "" {
		url += "&" + s.Scoreboard
	}
	if err := s.client.GetTransient(ctx, url, &res); err != nil {
		return nil, err
	}

	status := map[string]string{"pre": ingest.GameScheduled, "in": ingest.GameLive, "post": ingest.GameFinal}
	var games []ingest.Game
	for _, e := range res.Events {
		starts, err := time.Parse("2006-01-02T15:04Z07:00", e.Date)
		if err != nil || len(e.Competitions) == 0 {
			continue
		}
		game := ingest.Game{ProviderID: e.ID, StartsAt: starts, Status: status[e.Status.Type.State]}
		if game.Status == "" {
			game.Status = ingest.GameScheduled
		}
		if game.Status != ingest.GameScheduled {
			game.Detail = e.Status.Type.ShortDetail
		}
		for _, c := range e.Competitions[0].Competitors {
			score, _ := strconv.Atoi(c.Score)
			if c.HomeAway == "home" {
				game.HomeTeam, game.HomeScore = c.Team.ID, score
			} else {
				game.AwayTeam, game.AwayScore = c.Team.ID, score
			}
		}
		games = append(games, game)
	}
	return games, nil
}

// BoxScore reads each player's line from the game summary. ESPN groups
// stats (passing, rushing, ...) and writes pairs as one value ("12-22" for
// made-attempted); both are flattened into canonical keys.
func (s *Source) BoxScore(ctx context.Context, game ingest.Game) ([]ingest.StatLine, error) {
	var res struct {
		Boxscore struct {
			Players []struct {
				Statistics []struct {
					Name     string   `json:"name"`
					Keys     []string `json:"keys"`
					Athletes []struct {
						Athlete struct {
							ID string `json:"id"`
						} `json:"athlete"`
						Stats []string `json:"stats"`
					} `json:"athletes"`
				} `json:"statistics"`
			} `json:"players"`
		} `json:"boxscore"`
	}
	url := fmt.Sprintf("%s/%s/summary?event=%s", s.Base, s.Path, game.ProviderID)
	if err := s.client.GetTransient(ctx, url, &res); err != nil {
		return nil, err
	}

	byPlayer := map[string]map[string]float64{}
	var order []string // players in the order first seen, for stable output
	for _, team := range res.Boxscore.Players {
		for _, group := range team.Statistics {
			for _, a := range group.Athletes {
				for i, key := range group.Keys {
					if i >= len(a.Stats) {
						break
					}
					for name, value := range split(key, a.Stats[i]) {
						canonical, ok := s.Stats[group.Name+"."+name]
						if !ok {
							canonical, ok = s.Stats[name]
						}
						if !ok {
							continue
						}
						if byPlayer[a.Athlete.ID] == nil {
							byPlayer[a.Athlete.ID] = map[string]float64{}
							order = append(order, a.Athlete.ID)
						}
						byPlayer[a.Athlete.ID][canonical] += value
					}
				}
			}
		}
	}

	var lines []ingest.StatLine
	for _, id := range order {
		if s.Derive != nil {
			s.Derive(byPlayer[id])
		}
		lines = append(lines, ingest.StatLine{Provider: s.Provider, ProviderID: id, Stats: byPlayer[id]})
	}
	return lines, nil
}

// split turns one ESPN stat into its numbers: "points" and "13" is one
// stat, while "fieldGoalsMade-fieldGoalsAttempted" and "4-7" is two.
// Values that are not numbers are dropped.
func split(key, value string) map[string]float64 {
	names, values := []string{key}, []string{value}
	for _, separator := range []string{"-", "/"} {
		if strings.Contains(key, separator) {
			names, values = strings.Split(key, separator), strings.Split(value, separator)
		}
	}
	stats := map[string]float64{}
	for i, name := range names {
		if i >= len(values) {
			break
		}
		number := strings.ReplaceAll(strings.TrimPrefix(values[i], "+"), ",", "") // "+9", "1,039"
		if n, err := strconv.ParseFloat(number, 64); err == nil {
			stats[name] = n
		}
	}
	return stats
}

// LatestSeason is ESPN's number for the season being played, or the last
// one played. Leagues that run across the new year are numbered by the year
// they end in.
func (s *Source) LatestSeason(now time.Time) int {
	started := now.Month() >= s.SeasonStarts
	switch {
	case s.SeasonSpansYears && started:
		return now.Year() + 1
	case s.SeasonSpansYears || started:
		return now.Year()
	default:
		return now.Year() - 1
	}
}

// SeasonStats reads every player's regular-season totals for one season,
// a page of the league at a time.
func (s *Source) SeasonStats(ctx context.Context, year int) ([]ingest.SeasonLine, error) {
	var lines []ingest.SeasonLine
	for page, pages := 1, 1; page <= pages; page++ {
		var res struct {
			Pagination struct {
				Pages int `json:"pages"`
			} `json:"pagination"`
			RequestedSeason struct {
				DisplayName string `json:"displayName"`
			} `json:"requestedSeason"`
			// The names of each group's stats, in the order every athlete's totals follow.
			Categories []struct {
				Name  string   `json:"name"`
				Names []string `json:"names"`
			} `json:"categories"`
			Athletes []struct {
				Athlete struct {
					ID            string `json:"id"`
					TeamShortName string `json:"teamShortName"`
				} `json:"athlete"`
				Categories []struct {
					Name   string   `json:"name"`
					Totals []string `json:"totals"`
				} `json:"categories"`
			} `json:"athletes"`
		}
		// seasontype 2 is the regular season; without isqualified=false only the leaders are listed.
		url := fmt.Sprintf("%s/%s/statistics/byathlete?season=%d&seasontype=2&isqualified=false&limit=500&page=%d",
			s.WebBase, s.Path, year, page)
		if err := s.client.GetTransient(ctx, url, &res); err != nil {
			return nil, err
		}
		pages = res.Pagination.Pages

		names := map[string][]string{}
		for _, group := range res.Categories {
			names[group.Name] = group.Names
		}
		for _, a := range res.Athletes {
			line := ingest.SeasonLine{Provider: s.Provider, ProviderID: a.Athlete.ID, Season: ingest.Season{
				Year: year, Label: res.RequestedSeason.DisplayName, Team: a.Athlete.TeamShortName, Stats: map[string]float64{},
			}}
			averages := map[string]float64{} // per game, by canonical stat
			for _, group := range a.Categories {
				counted := slices.Contains(s.SeasonGroups, group.Name)
				for i, name := range names[group.Name] {
					if i >= len(group.Totals) {
						break
					}
					for stat, value := range split(name, group.Totals[i]) {
						if stat == "gamesPlayed" {
							line.Games = int(value)
							continue
						}
						if !counted {
							continue
						}
						if base, isAverage := strings.CutPrefix(stat, "avg"); isAverage && base != "" {
							if canonical, ok := s.Stats[strings.ToLower(base[:1])+base[1:]]; ok {
								averages[canonical] = value
							}
							continue
						}
						canonical, ok := s.Stats[group.Name+"."+stat]
						if !ok {
							canonical, ok = s.Stats[stat]
						}
						if ok {
							line.Stats[canonical] += value
						}
					}
				}
			}
			// Some leagues (the WNBA) list most stats only per game. The total
			// is then rebuilt from the average, which the feed rounds to one
			// decimal, so it can be off by a few over a season.
			for canonical, average := range averages {
				if _, listed := line.Stats[canonical]; !listed {
					line.Stats[canonical] = math.Round(average * float64(line.Games))
				}
			}
			if line.Games > 0 {
				lines = append(lines, line)
			}
		}
	}
	return lines, nil
}

const (
	defaultCoreBase = "https://sports.core.api.espn.com/v2/sports"
	recruitProvider = "espn_recruit" // recruit ids are a different id space from athlete ids
)

// recruitClasses is how many high school classes are listed, soonest first.
const recruitClasses = 3

// Recruits lists the high school classes that will enter college next.
type Recruits struct {
	Base   string // overridable in tests
	Year   int    // first graduating class; 0 means the next class to enroll
	client *ingest.Client
}

func NewRecruits(client *ingest.Client) *Recruits {
	return &Recruits{Base: defaultCoreBase, client: client}
}

func (r *Recruits) Prospects(ctx context.Context) ([]ingest.Player, error) {
	first := r.Year
	if first == 0 {
		// A class enrolls in August; from then on the next class is the one to draft.
		first = time.Now().Year()
		if time.Now().Month() >= time.August {
			first++
		}
	}

	var players []ingest.Player
	for year := first; year < first+recruitClasses; year++ {
		class, err := r.class(ctx, year)
		if ingest.NotFound(err) {
			break // not published yet
		}
		if err != nil {
			return nil, err
		}
		players = append(players, class...)
	}
	return players, nil
}

func (r *Recruits) class(ctx context.Context, year int) ([]ingest.Player, error) {
	var players []ingest.Player
	for page, pages := 1, 1; page <= pages; page++ {
		var res struct {
			PageCount int `json:"pageCount"`
			Items     []struct {
				Athlete struct {
					ID       string `json:"id"`
					FullName string `json:"fullName"`
					Position struct {
						Abbreviation string `json:"abbreviation"`
					} `json:"position"`
					HighSchool struct {
						Name string `json:"name"`
					} `json:"highSchool"`
				} `json:"athlete"`
			} `json:"items"`
		}
		url := fmt.Sprintf("%s/basketball/leagues/mens-college-basketball/recruiting/%d/athletes?limit=500&page=%d", r.Base, year, page)
		if err := r.client.GetJSON(ctx, url, &res); err != nil {
			return nil, err
		}
		pages = res.PageCount

		for _, item := range res.Items {
			a := item.Athlete
			if a.ID == "" {
				continue
			}
			p := ingest.Player{
				Provider:   recruitProvider,
				ProviderID: a.ID,
				FullName:   a.FullName,
				Note:       fmt.Sprintf("Class of %d", year),
			}
			if a.HighSchool.Name != "" {
				p.Note += ", " + a.HighSchool.Name
			}
			if a.Position.Abbreviation != "" {
				p.Positions = []string{a.Position.Abbreviation}
			}
			players = append(players, p)
		}
	}
	return players, nil
}

// Draft lists a pro league's draft prospects: the class ESPN ranks for the
// next draft, and the picks of recent drafts, which keeps a drafted player
// who has not joined a roster (one stashed overseas, say) in the pool.
// Each is stored under his ordinary athlete id, so he is the same row when
// he reaches a roster, and a college player we already hold is left alone.
type Draft struct {
	Base     string     // overridable in tests
	Path     string     // e.g. "basketball/leagues/nba"
	Provider string     // the league's athlete id space
	Month    time.Month // when the draft is held
	Classes  int        // how many past drafts' picks are listed
	// Positions, when set, keeps only prospects at these positions.
	Positions []string
	client    *ingest.Client
}

func NewDraft(client *ingest.Client, draft Draft) *Draft {
	draft.Base, draft.client = defaultCoreBase, client
	return &draft
}

func (d *Draft) Prospects(ctx context.Context) ([]ingest.Player, error) {
	next := time.Now().Year()
	if time.Now().Month() > d.Month {
		next++
	}

	var players []ingest.Player
	for year := next - d.Classes; year < next; year++ {
		var res struct {
			Items []struct {
				Picks []struct {
					Overall int `json:"overall"`
					Athlete ref `json:"athlete"`
				} `json:"picks"`
			} `json:"items"`
		}
		if err := d.client.GetJSON(ctx, fmt.Sprintf("%s/%s/seasons/%d/draft/rounds", d.Base, d.Path, year), &res); err != nil {
			return nil, err
		}
		for _, round := range res.Items {
			for _, pick := range round.Picks {
				p, err := d.athlete(ctx, year, pick.Athlete.id())
				if err != nil {
					return nil, err
				}
				if p != nil {
					p.Note = fmt.Sprintf("%d draft, pick %d", year, pick.Overall)
					players = append(players, *p)
				}
			}
		}
	}

	var board struct {
		Items []ref `json:"items"`
	}
	err := d.client.GetJSON(ctx, fmt.Sprintf("%s/%s/seasons/%d/draft/athletes?limit=1000", d.Base, d.Path, next), &board)
	if ingest.NotFound(err) {
		return players, nil // not published yet
	}
	if err != nil {
		return nil, err
	}
	for _, item := range board.Items {
		p, err := d.athlete(ctx, next, item.id())
		if err != nil {
			return nil, err
		}
		if p != nil {
			players = append(players, *p)
		}
	}
	return players, nil
}

// athlete reads one draft entry. The draft numbers its entries separately
// from athletes, so the entry is followed to the athlete it describes; nil
// means it names nobody we could recognise later.
func (d *Draft) athlete(ctx context.Context, year int, entry string) (*ingest.Player, error) {
	if entry == "" { // a pick not yet made
		return nil, nil
	}
	var res struct {
		FullName string `json:"fullName"`
		Athlete  ref    `json:"athlete"`
		Position struct {
			Abbreviation string `json:"abbreviation"`
		} `json:"position"`
		Attributes []struct {
			Name         string `json:"name"`
			DisplayValue string `json:"displayValue"`
		} `json:"attributes"`
	}
	if err := d.client.GetJSON(ctx, fmt.Sprintf("%s/%s/seasons/%d/draft/athletes/%s", d.Base, d.Path, year, entry), &res); err != nil {
		return nil, err
	}
	id := res.Athlete.id()
	if id == "" || res.FullName == "" {
		return nil, nil
	}
	p := &ingest.Player{
		Provider:   d.Provider,
		ProviderID: id,
		FullName:   html.UnescapeString(res.FullName), // this feed escapes accented letters
		Note:       fmt.Sprintf("%d draft prospect", year),
	}
	for _, a := range res.Attributes {
		if a.Name == "overall" && a.DisplayValue != "" && a.DisplayValue != "0" {
			p.Note += ", ranked " + a.DisplayValue
		}
	}
	if len(d.Positions) > 0 && !slices.Contains(d.Positions, res.Position.Abbreviation) {
		return nil, nil
	}
	if res.Position.Abbreviation != "" {
		p.Positions = []string{res.Position.Abbreviation}
	}
	return p, nil
}

// ref is a link to another record in ESPN's core API.
type ref struct {
	URL string `json:"$ref"`
}

// id is the last path segment of the link: the record's id.
func (r ref) id() string {
	path, _, _ := strings.Cut(r.URL, "?")
	return path[strings.LastIndex(path, "/")+1:]
}
