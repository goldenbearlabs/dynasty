package nbamodel

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// FeatureSet makes every experiment reproducible without changing the frozen
// baseline. Availability below is inferred from PRIOR games, not target DNPs.
type FeatureSet struct {
	TeammateMinutes      bool `json:"teammate_minutes,omitempty"`
	TeammateRates        bool `json:"teammate_rates,omitempty"`
	TeamBudget           bool `json:"team_budget,omitempty"`
	Workload             bool `json:"workload,omitempty"`
	Offseason            bool `json:"offseason,omitempty"`
	Pace                 bool `json:"pace,omitempty"`
	Competitiveness      bool `json:"competitiveness,omitempty"`
	RoleState            bool `json:"role_state,omitempty"`
	StatWindows          bool `json:"stat_windows,omitempty"`
	Opportunities        bool `json:"opportunities,omitempty"`
	ReportedAvailability bool `json:"reported_availability,omitempty"`
}

func ParseFeatureSet(names string) (FeatureSet, error) {
	var f FeatureSet
	for _, name := range strings.Split(names, ",") {
		switch strings.TrimSpace(name) {
		case "", "baseline":
		case "teammate_minutes":
			f.TeammateMinutes = true
		case "teammate_rates":
			f.TeammateRates = true
		case "team_budget":
			f.TeamBudget = true
		case "workload":
			f.Workload = true
		case "offseason":
			f.Offseason = true
		case "pace":
			f.Pace = true
		case "competitiveness":
			f.Competitiveness = true
		case "role_state":
			f.RoleState = true
		case "stat_windows":
			f.StatWindows = true
		case "opportunities":
			f.Opportunities = true
		case "reported_availability":
			f.ReportedAvailability = true
		default:
			return f, fmt.Errorf("unknown feature group %q", name)
		}
	}
	return f, nil
}

func minuteFeatureNames(f FeatureSet) []string {
	n := slices.Clone(MinuteFeatures)
	if f.TeammateMinutes {
		n = append(n, "missing_teammate_minutes", "missing_position_minutes", "expected_rotation_minutes", "estimated_presence", "rotation_size")
	}
	if f.Workload {
		n = append(n, "team_rest", "team_games_4_days", "team_games_7_days", "player_minutes_3_days", "player_minutes_7_days", "return_gap")
	}
	if f.Offseason {
		n = append(n, "offseason_reset_share", "previous_season_minutes_gap")
	}
	if f.Competitiveness {
		n = append(n, "expected_margin_magnitude", "starter_margin_interaction")
	}
	return append(n, priorityMinuteNames(f)...)
}

func rateFeatureNames(f FeatureSet) []string {
	n := slices.Clone(RateFeatures)
	if f.TeammateRates {
		n = append(n, "missing_teammate_points", "missing_teammate_assists", "missing_teammate_rebounds", "missing_position_minutes", "estimated_presence")
	}
	if f.Workload {
		n = append(n, "team_rest", "team_games_4_days", "player_minutes_3_days", "player_minutes_7_days")
	}
	if f.Offseason {
		n = append(n, "offseason_reset_share")
	}
	if f.Pace {
		n = append(n, "expected_pace_delta", "pace_change_for_team", "opponent_pace_delta")
	}
	return append(n, priorityRateNames(f)...)
}

type teamGame struct {
	ID, Team, Opponent                           string
	At                                           time.Time
	Season                                       int
	Minutes, Points, Possessions, OpponentPoints float64
	Players                                      map[string]bool
}

type rotationPlayer struct {
	ID, Position      string
	Presence, Minutes float64
	Rates             map[string]float64
}

type teamContext struct {
	Players                                          []rotationPlayer
	Rest, Games4, Games7, Pace, OpponentPace, Margin float64
	ExpectedMinutes                                  float64
}

type teamState struct {
	Roster   map[string]Row
	Games    map[string][]teamGame
	Contexts map[string]*teamContext
	Budgets  map[string]*BudgetAllocation
}

type ContextFeatures struct {
	EstimatedPresence       float64 `json:"estimated_presence"`
	RosterKnown             bool    `json:"roster_known"`
	MissingTeammateMinutes  float64 `json:"missing_teammate_minutes"`
	MissingPositionMinutes  float64 `json:"missing_position_minutes"`
	MissingPoints           float64 `json:"missing_points"`
	MissingAssists          float64 `json:"missing_assists"`
	MissingRebounds         float64 `json:"missing_rebounds"`
	ExpectedRotationMinutes float64 `json:"expected_rotation_minutes"`
	TeamRestDays            float64 `json:"team_rest_days"`
	ExpectedPossessions     float64 `json:"expected_possessions"`
	ExpectedMargin          float64 `json:"expected_margin_proxy"`
	OffseasonReset          float64 `json:"offseason_reset_share"`
}

func (s *State) observeTeams(rows []Row) {
	if s.team == nil {
		s.team = &teamState{Roster: map[string]Row{}, Games: map[string][]teamGame{}}
	}
	s.team.Contexts = map[string]*teamContext{}
	s.team.Budgets = map[string]*BudgetAllocation{}
	games := map[string]*teamGame{}
	var order []string
	for _, r := range rows {
		s.team.Roster[r.PlayerID] = r
		key := r.GameID + "/" + r.Team
		if games[key] == nil {
			games[key] = &teamGame{ID: r.GameID, Team: r.Team, Opponent: r.Opponent, At: r.StartsAt, Season: r.Season, Players: map[string]bool{}}
			order = append(order, key)
		}
		g := games[key]
		g.Players[r.PlayerID] = r.Played && r.Minutes > 0
		if !r.Played || r.Minutes <= 0 {
			continue
		}
		g.Minutes += r.Minutes
		g.Points += r.Stats["pts"]
		g.Possessions += r.Stats["fga"] + .44*r.Stats["fta"] - r.Stats["oreb"] + r.Stats["tov"]
	}
	for _, key := range order {
		g := games[key]
		if other := games[g.ID+"/"+g.Opponent]; other != nil {
			g.OpponentPoints = other.Points
		}
		if g.Minutes > 0 {
			g.Possessions *= 240 / g.Minutes
		}
		s.team.Games[g.Team] = append(s.team.Games[g.Team], *g)
		if len(s.team.Games[g.Team]) > 180 {
			s.team.Games[g.Team] = s.team.Games[g.Team][1:]
		}
	}
}

func (s *State) teamHistory(team string) []teamGame {
	if s.team == nil {
		return nil
	}
	return s.team.Games[team]
}

func teamMeans(history []teamGame) (pace, net float64) {
	var weight float64
	for i := len(history) - 1; i >= 0 && len(history)-1-i < 30; i-- {
		w := math.Exp2(-float64(len(history)-1-i) / 10)
		pace += w * history[i].Possessions
		net += w * (history[i].Points - history[i].OpponentPoints)
		weight += w
	}
	// Five games of a neutral prior keep opening/unknown teams stable.
	return (pace + 5*100) / (weight + 5), net / (weight + 5)
}

func (s *State) context(t Target, c Config) *teamContext {
	if s.team == nil {
		s.observeTeams(nil)
	}
	key := fmt.Sprintf("%s/%s/%.0f", t.GameID, t.Team, c.HalfLife)
	if cached := s.team.Contexts[key]; cached != nil {
		return cached
	}
	x := &teamContext{Rest: 7}
	history := s.teamHistory(t.Team)
	if len(history) > 0 {
		x.Rest = min(14, max(0, t.StartsAt.Sub(history[len(history)-1].At).Hours()/24))
	}
	for _, g := range history {
		hours := t.StartsAt.Sub(g.At).Hours()
		if hours > 0 && hours <= 4*24 {
			x.Games4++
		}
		if hours > 0 && hours <= 7*24 {
			x.Games7++
		}
	}
	ownPace, ownNet := teamMeans(history)
	opponentPace, opponentNet := teamMeans(s.teamHistory(t.Opponent))
	x.Pace = (ownPace + opponentPace) / 2
	x.OpponentPace = opponentPace
	x.Margin = (ownNet - opponentNet) / 2
	if c.Features.TeammateMinutes || c.Features.TeammateRates || c.Features.TeamBudget || c.Features.ReportedAvailability {
		var ids []string
		for id, r := range s.team.Roster {
			if r.Team == t.Team && r.Season >= t.Season-1 && t.StartsAt.Sub(r.StartsAt) <= 210*24*time.Hour {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		baseConfig := c
		baseConfig.Features = FeatureSet{}
		for _, id := range ids {
			prior := t
			prior.PlayerID = id
			f := s.Build(prior, baseConfig)
			var appears, weight float64
			for i := len(history) - 1; i >= 0 && len(history)-1-i < 8; i-- {
				w := math.Pow(.8, float64(len(history)-1-i))
				weight += w
				if history[i].Players[id] {
					appears += w
				}
			}
			q := (appears + .5) / (weight + 1)
			p := rotationPlayer{ID: id, Position: f.Position, Presence: q, Minutes: f.BaseMinutes, Rates: f.BaseRates}
			if c.Features.ReportedAvailability {
				if report := s.reports[t.GameID]; report != nil && report.PublishedAt.Before(t.StartsAt) {
					if team, ok := report.Teams[t.Team]; ok && team.Submitted {
						for _, injury := range team.Players {
							if injury.PlayerID == id {
								switch injury.Status {
								case "Out":
									p.Presence = 0
								case "Doubtful":
									p.Presence *= .25
								case "Questionable":
									p.Presence *= .5
								case "Probable":
									p.Presence *= .9
								}
							}
						}
					}
				}
				p.Minutes = f.Minutes[2] * 36
			}
			x.Players = append(x.Players, p)
			x.ExpectedMinutes += p.Presence * p.Minutes
		}
	}
	s.team.Contexts[key] = x
	return x
}

func (s *State) extend(t Target, c Config, f *Features) {
	x := s.context(t, c)
	d := &ContextFeatures{EstimatedPresence: .5, ExpectedRotationMinutes: x.ExpectedMinutes, TeamRestDays: x.Rest, ExpectedPossessions: x.Pace, ExpectedMargin: x.Margin}
	f.Context = d
	for _, p := range x.Players {
		if p.ID == t.PlayerID {
			d.EstimatedPresence = p.Presence
			d.RosterKnown = true
			continue
		}
		missing := (1 - p.Presence) * p.Minutes
		d.MissingTeammateMinutes += missing
		if p.Position == f.Position {
			d.MissingPositionMinutes += missing
		}
		d.MissingPoints += missing * p.Rates["pts"]
		d.MissingAssists += missing * p.Rates["ast"]
		d.MissingRebounds += missing * p.Rates["reb"]
	}
	if c.Features.Offseason {
		var minutes, games float64
		for _, r := range s.players[t.PlayerID] {
			if r.Season == t.Season-1 {
				minutes += r.Minutes
				games++
			}
		}
		if games >= 10 && f.SeasonGames < 10 {
			a := float64(10-f.SeasonGames) / 10
			previous := minutes / games
			oldBase := f.BaseMinutes
			d.OffseasonReset = a
			f.BaseMinutes = (1-a)*oldBase + a*previous
			for _, i := range []int{1, 2, 3} {
				f.Minutes[i] = (1-a)*f.Minutes[i] + a*previous/36
			}
		}
	}
	if c.Features.TeammateMinutes {
		f.Minutes = append(f.Minutes, d.MissingTeammateMinutes/120, d.MissingPositionMinutes/60, x.ExpectedMinutes/240, d.EstimatedPresence, float64(len(x.Players))/15)
	}
	var minutes3, minutes7, gap float64
	history := s.players[t.PlayerID]
	if len(history) > 0 {
		gap = min(30, max(0, t.StartsAt.Sub(history[len(history)-1].StartsAt).Hours()/24))
	}
	for _, r := range history {
		hours := t.StartsAt.Sub(r.StartsAt).Hours()
		if hours > 0 && hours <= 3*24 {
			minutes3 += r.Minutes
		}
		if hours > 0 && hours <= 7*24 {
			minutes7 += r.Minutes
		}
	}
	if c.Features.Workload {
		f.Minutes = append(f.Minutes, x.Rest/7, x.Games4/4, x.Games7/7, minutes3/100, minutes7/200, gap/30)
	}
	if c.Features.Offseason {
		f.Minutes = append(f.Minutes, d.OffseasonReset, (f.Minutes[4]*36-f.BaseMinutes)/36)
	}
	if c.Features.Competitiveness {
		magnitude := min(30, math.Abs(x.Margin)) / 20
		f.Minutes = append(f.Minutes, magnitude, magnitude*f.Minutes[5])
	}
	ownPace, _ := teamMeans(s.teamHistory(t.Team))
	for _, stat := range Stats {
		v := f.Rates[stat]
		if c.Features.Offseason {
			v[8] = f.Minutes[1] - f.Minutes[2]
		}
		if c.Features.TeammateRates {
			v = append(v, d.MissingPoints/100, d.MissingAssists/30, d.MissingRebounds/40, d.MissingPositionMinutes/60, d.EstimatedPresence)
		}
		if c.Features.Workload {
			v = append(v, x.Rest/7, x.Games4/4, minutes3/100, minutes7/200)
		}
		if c.Features.Offseason {
			v = append(v, d.OffseasonReset)
		}
		if c.Features.Pace {
			v = append(v, (x.Pace-100)/100, (x.Pace-ownPace)/100, (x.OpponentPace-100)/100)
		}
		f.Rates[stat] = v
	}
	s.extendPriorities(t, c, f)
}

// BudgetAllocation constrains EXPECTED minutes: sum(present_probability *
// conditional_minutes) + unknown_rotation_reserve == 240. Conditional minutes
// alone need not sum to 240. This is not a confirmed-active-lineup forecast.
type BudgetAllocation struct {
	Minutes  map[string]float64 `json:"conditional_minutes"`
	Presence map[string]float64 `json:"estimated_presence"`
	Reserve  float64            `json:"unknown_rotation_minutes"`
	Total    float64            `json:"expected_team_minutes"`
}

func allocateMinutes(raw, presence map[string]float64, reserveWeight float64) *BudgetAllocation {
	var ids []string
	for id := range raw {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	total := func(scale float64) float64 {
		n := reserveWeight * scale
		for _, id := range ids {
			n += presence[id] * min(48, raw[id]*scale)
		}
		return n
	}
	lo, hi := 0.0, 1.0
	for total(hi) < 240 {
		hi *= 2
	}
	for range 70 {
		mid := (lo + hi) / 2
		if total(mid) < 240 {
			lo = mid
		} else {
			hi = mid
		}
	}
	scale := (lo + hi) / 2
	a := &BudgetAllocation{Minutes: map[string]float64{}, Presence: presence, Reserve: reserveWeight * scale}
	a.Total = a.Reserve
	for _, id := range ids {
		a.Minutes[id] = min(48, raw[id]*scale)
		a.Total += presence[id] * a.Minutes[id]
	}
	return a
}

func (m Model) Forecast(t Target, f Features, s *State, scoring map[string]float64) Prediction {
	if !m.Config.Features.TeamBudget {
		return m.Predict(f, scoring)
	}
	x := s.context(t, m.Config)
	key := fmt.Sprintf("%s/%s/%v/%s", t.GameID, t.Team, m.Config, m.TrainedBefore)
	a := s.team.Budgets[key]
	if a == nil {
		raw, probability := map[string]float64{}, map[string]float64{}
		for _, p := range x.Players {
			other := t
			other.PlayerID = p.ID
			features := s.Build(other, m.Config)
			raw[p.ID] = m.Predict(features, scoring).Minutes
			probability[p.ID] = p.Presence
		}
		a = allocateMinutes(raw, probability, 8)
		s.team.Budgets[key] = a
	}
	if minutes, ok := a.Minutes[t.PlayerID]; ok {
		return m.project(f, minutes, scoring)
	}
	// A first-ever appearance cannot reveal its player identity to the team's
	// historical roster. Retain the cold-start forecast and report no coverage.
	return m.Predict(f, scoring)
}
