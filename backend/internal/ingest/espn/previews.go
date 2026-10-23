package espn

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"crossover/internal/ingest"
)

// Previews reads a day's scoreboard for the betting line on each game and
// the probable starting goalies or pitchers. ESPN marks a goalie Expected
// until his team confirms him; a pitcher carries no status and stays expected.
func (s *Source) Previews(ctx context.Context, day time.Time) ([]ingest.Preview, error) {
	type price struct {
		Close struct {
			Odds string `json:"odds"` // American: "-142", "+120"
		} `json:"close"`
	}
	var res struct {
		Events []struct {
			ID           string `json:"id"`
			Competitions []struct {
				Competitors []struct {
					HomeAway string `json:"homeAway"`
					Team     struct {
						Name string `json:"name"` // the nickname: "Red Sox"
					} `json:"team"`
					Probables []struct {
						Name    string `json:"name"` // probableStartingGoalie, probableStartingPitcher
						Athlete struct {
							ID       string `json:"id"`
							FullName string `json:"fullName"`
						} `json:"athlete"`
						Status struct {
							Type string `json:"type"` // expected | confirmed
						} `json:"status"`
					} `json:"probables"`
				} `json:"competitors"`
				Odds []struct {
					OverUnder float64 `json:"overUnder"`
					Spread    float64 `json:"spread"` // the home side's
					Moneyline struct {
						Home price `json:"home"`
						Away price `json:"away"`
					} `json:"moneyline"`
				} `json:"odds"`
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
	// Hockey and baseball spreads are a fixed line of 1.5, so how the total
	// splits between the sides comes from the moneyline instead.
	fixedLine := strings.HasPrefix(s.Path, "hockey/") || strings.HasPrefix(s.Path, "baseball/")

	var previews []ingest.Preview
	for _, e := range res.Events {
		if len(e.Competitions) == 0 {
			continue
		}
		c := e.Competitions[0]
		p := ingest.Preview{GameID: e.ID}
		for _, team := range c.Competitors {
			if team.HomeAway == "home" {
				p.HomeTeam = team.Team.Name
			}
			for _, pr := range team.Probables {
				if !strings.HasPrefix(pr.Name, "probableStarting") || pr.Athlete.FullName == "" {
					continue
				}
				p.Starters = append(p.Starters, ingest.Starter{
					Provider: s.Provider, ProviderID: pr.Athlete.ID, FullName: pr.Athlete.FullName, Confirmed: pr.Status.Type == "confirmed",
				})
			}
		}
		if len(c.Odds) > 0 && c.Odds[0].OverUnder > 0 {
			o := c.Odds[0]
			p.Odds = gameOdds(o.OverUnder, o.Spread, chance(o.Moneyline.Home.Close.Odds, o.Moneyline.Away.Close.Odds), fixedLine)
		}
		previews = append(previews, p)
	}
	return previews, nil
}

// gameOdds splits a game's total between the sides: by the spread, or, on
// a fixed line, by the chance of winning. A side expected to win 65% of
// the time takes about 57% of the total in hockey and baseball. homeWin is
// zero when there is no moneyline.
func gameOdds(total, spread, homeWin float64, fixedLine bool) *ingest.Odds {
	home := (total - spread) / 2
	if fixedLine {
		p := homeWin
		if p == 0 {
			p = 0.5
		}
		home = total * (0.5 + 0.45*(p-0.5))
	}
	return &ingest.Odds{HomeScore: home, AwayScore: total - home, HomeWin: homeWin}
}

// chance is the home side's chance of winning from both moneylines, with
// the bookmaker's margin taken out. Zero when either is missing.
func chance(home, away string) float64 {
	h, a := implied(home), implied(away)
	if h == 0 || a == 0 {
		return 0
	}
	return h / (h + a)
}

// implied is the chance an American price stands for: -150 is 60%, +150 is 40%.
func implied(american string) float64 {
	n, err := strconv.ParseFloat(strings.TrimPrefix(american, "+"), 64)
	switch {
	case err != nil || n == 0:
		return 0
	case n < 0:
		return -n / (-n + 100)
	default:
		return 100 / (n + 100)
	}
}
