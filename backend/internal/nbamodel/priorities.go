package nbamodel

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"time"
)

type ReportedPlayer struct {
	PlayerID string `json:"player_id"`
	Status   string `json:"status"`
}
type ReportedTeam struct {
	Submitted bool             `json:"submitted"`
	Players   []ReportedPlayer `json:"players"`
}
type PregameReport struct {
	PublishedAt time.Time               `json:"published_at"`
	StartsAt    time.Time               `json:"starts_at"`
	Teams       map[string]ReportedTeam `json:"teams"`
}

type OfficialFeatures struct {
	Available          bool       `json:"team_report_available"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	PlayerStatus       string     `json:"player_status,omitempty"`
	OutTeammateMinutes float64    `json:"out_teammate_recent_minutes"`
}

func ReadPregameReports(reader io.Reader, rows []Row) (map[string]*PregameReport, error) {
	var source struct {
		Reports map[string]*PregameReport `json:"reports"`
	}
	if err := json.NewDecoder(reader).Decode(&source); err != nil {
		return nil, err
	}
	for id, r := range source.Reports {
		if r == nil || r.PublishedAt.IsZero() || r.StartsAt.IsZero() || !r.PublishedAt.Before(r.StartsAt) {
			return nil, fmt.Errorf("invalid/non-pregame report for %s", id)
		}
	}
	for i := range rows {
		if r := source.Reports[rows[i].GameID]; r != nil {
			if !r.PublishedAt.Before(rows[i].StartsAt) || r.StartsAt.Sub(rows[i].StartsAt).Abs() > time.Minute {
				return nil, fmt.Errorf("report/game timestamp mismatch for %s", rows[i].GameID)
			}
			rows[i].Pregame = r
		}
	}
	return source.Reports, nil
}

func priorityMinuteNames(f FeatureSet) []string {
	var n []string
	if f.RoleState {
		n = append(n, "role_median_5", "role_median_15", "role_start_change", "role_persistent_shift", "prior_short_foul_share", "role_minutes_spread")
	}
	if f.ReportedAvailability {
		n = append(n, "official_report_available", "official_out_teammate_minutes", "official_player_status", "official_report_age")
	}
	return n
}
func priorityRateNames(f FeatureSet) []string {
	var n []string
	if f.RoleState {
		n = append(n, "role_shot_volume_change", "role_persistent_shift")
	}
	if f.ReportedAvailability {
		n = append(n, "official_report_available", "official_out_teammate_minutes", "official_player_status")
	}
	return n
}

func medianValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := slices.Clone(values)
	slices.Sort(v)
	n := len(v)
	if n%2 == 0 {
		return (v[n/2-1] + v[n/2]) / 2
	}
	return v[n/2]
}

func (s *State) extendPriorities(t Target, c Config, f *Features) {
	history := s.players[t.PlayerID]
	if c.Features.RoleState {
		var m5, m15, old []float64
		var start5, startOld, shots5, min5, shotsOld, minOld, shortFouls float64
		for i := len(history) - 1; i >= 0 && len(history)-1-i < 15; i-- {
			r := history[i]
			age := len(history) - 1 - i
			m15 = append(m15, r.Minutes)
			if age < 5 {
				m5 = append(m5, r.Minutes)
				min5 += r.Minutes
				shots5 += r.Stats["fga"]
				if r.Starter {
					start5++
				}
				if r.Stats["pf"] >= 5 && r.Minutes < f.BaseMinutes-6 {
					shortFouls++
				}
			}
			if age >= 5 {
				old = append(old, r.Minutes)
				minOld += r.Minutes
				shotsOld += r.Stats["fga"]
				if r.Starter {
					startOld++
				}
			}
		}
		med5, med15 := medianValue(m5), medianValue(m15)
		if len(m5) == 0 {
			med5 = f.BaseMinutes
			med15 = f.BaseMinutes
		}
		shift, shotChange, startChange, spread := 0.0, 0.0, 0.0, 0.0
		if len(m5) > 0 {
			startChange = start5 / float64(len(m5))
			shortFouls /= float64(len(m5))
			for _, v := range m5 {
				spread += math.Abs(v - med5)
			}
			spread /= float64(len(m5))
		}
		if len(old) > 0 {
			startChange -= startOld / float64(len(old))
			shift = (med5 - medianValue(old)) / 36
		}
		if min5 > 0 && minOld > 0 {
			shotChange = shots5/min5 - shotsOld/minOld
		}
		if len(m5) < 3 || len(old) < 3 {
			shift = 0
		}
		f.Minutes = append(f.Minutes, med5/36, med15/36, startChange, shift, shortFouls, spread/12)
		for _, stat := range Stats {
			f.Rates[stat] = append(f.Rates[stat], shotChange, shift)
		}
	}
	if c.Features.ReportedAvailability {
		f.Official = &OfficialFeatures{}
		available, out, status, age := 0.0, 0.0, 0.0, 0.0
		if report := s.reports[t.GameID]; report != nil && report.PublishedAt.Before(t.StartsAt) {
			published := report.PublishedAt
			f.Official.PublishedAt = &published
			if team, ok := report.Teams[t.Team]; ok && team.Submitted {
				available = 1
				f.Official.Available = true
				age = min(24, t.StartsAt.Sub(report.PublishedAt).Hours()) / 24
				for _, p := range team.Players {
					if p.PlayerID == t.PlayerID {
						f.Official.PlayerStatus = p.Status
						switch p.Status {
						case "Out":
							status = -1
						case "Doubtful":
							status = -.75
						case "Questionable":
							status = -.5
						case "Probable":
							status = -.1
						case "Available":
							status = 1
						}
						continue
					}
					if p.Status == "Out" {
						h := s.players[p.PlayerID]
						var minutes float64
						n := 0
						for i := len(h) - 1; i >= 0 && n < 10; i-- {
							minutes += h[i].Minutes
							n++
						}
						if n > 0 {
							out += minutes / float64(n)
						}
					}
				}
			}
		}
		f.Official.OutTeammateMinutes = out
		f.Minutes = append(f.Minutes, available, out/120, status, age)
		for _, stat := range Stats {
			f.Rates[stat] = append(f.Rates[stat], available, out/120, status)
		}
	}
	if c.Features.Opportunities {
		f.Shooting = s.shooting(t, c, f.Position)
	}
}

func (s *State) statSpecificRates(t Target, c Config, f *Features) {
	history := s.players[t.PlayerID]
	_, rates := s.priors(t.Season, f.Position)
	for _, stat := range Stats {
		half, prior := c.HalfLife, c.PriorMinutes
		switch stat {
		case "fga", "fta", "tov", "ast":
			half *= .5
			prior *= .5
		case "stl", "blk":
			half *= 2
			prior *= 3
		case "dd", "td", "pts40", "pts50":
			half *= 2
			prior *= 6
		}
		var minutes, total, recentMin, recentTotal, seasonMin, seasonTotal float64
		var meeting *Row
		for i := len(history) - 1; i >= 0; i-- {
			r := &history[i]
			if r.Season < t.Season-2 {
				break
			}
			age := len(history) - 1 - i
			w := math.Exp2(-float64(age)/half) * math.Pow(c.SeasonDecay, float64(t.Season-r.Season))
			minutes += w * r.Minutes
			total += w * r.Stats[stat]
			if age < 10 {
				recentMin += r.Minutes
				recentTotal += r.Stats[stat]
			}
			if r.Season == t.Season {
				seasonMin += r.Minutes
				seasonTotal += r.Stats[stat]
			}
			if meeting == nil && r.Opponent == t.Opponent && t.StartsAt.Sub(r.StartsAt) <= 365*24*time.Hour {
				meeting = r
			}
		}
		scale := f.Scales[stat]
		base := (total + prior*rates[stat]) / (minutes + prior)
		f.BaseRates[stat] = base
		v := f.Rates[stat]
		v[1] = base / scale
		if recentMin > 0 {
			v[2] = (recentTotal/recentMin - base) / scale
		}
		if seasonMin > 0 {
			v[3] = (seasonTotal/seasonMin - base) / scale * float64(f.SeasonGames) / (float64(f.SeasonGames) + 15)
		}
		v[4] = (f.OpponentFactors[stat] - 1) * base / scale
		if meeting != nil && meeting.Minutes > 0 {
			v[5] = (meeting.Stats[stat]/meeting.Minutes - base) / scale * meeting.Minutes / (meeting.Minutes + 120)
		}
	}
}

type ShootingFeatures struct {
	ThreeShare float64 `json:"three_attempt_share"`
	TwoPct     float64 `json:"two_point_pct"`
	ThreePct   float64 `json:"three_point_pct"`
	FreePct    float64 `json:"free_throw_pct"`
}

func (s *State) shooting(t Target, c Config, pos string) *ShootingFeatures {
	league := map[string]float64{}
	for year := t.Season - 2; year <= t.Season; year++ {
		total := s.positions[year][pos]
		if total == nil || total.Minutes < 300 {
			total = s.positions[year]["ALL"]
		}
		if total == nil {
			continue
		}
		for _, stat := range []string{"fga", "fgm", "tpa", "tpm", "fta", "ftm"} {
			league[stat] += math.Pow(.5, float64(t.Season-year)) * total.Stats[stat]
		}
	}
	ratio := func(a, b, fallback float64) float64 {
		if b <= 0 {
			return fallback
		}
		return min(1, max(0, a/b))
	}
	lShare := ratio(league["tpa"], league["fga"], .35)
	lTwo := ratio(league["fgm"]-league["tpm"], league["fga"]-league["tpa"], .52)
	lThree := ratio(league["tpm"], league["tpa"], .35)
	lFree := ratio(league["ftm"], league["fta"], .77)
	volume, skill := map[string]float64{}, map[string]float64{}
	history := s.players[t.PlayerID]
	for i := len(history) - 1; i >= 0; i-- {
		r := history[i]
		if r.Season < t.Season-2 {
			break
		}
		age := float64(len(history) - 1 - i)
		season := math.Pow(c.SeasonDecay, float64(t.Season-r.Season))
		for _, stat := range []string{"fga", "fgm", "tpa", "tpm", "fta", "ftm"} {
			volume[stat] += math.Exp2(-age/10) * season * r.Stats[stat]
			skill[stat] += math.Exp2(-age/75) * season * r.Stats[stat]
		}
	}
	return &ShootingFeatures{ratio(volume["tpa"]+50*lShare, volume["fga"]+50, lShare), ratio(skill["fgm"]-skill["tpm"]+200*lTwo, skill["fga"]-skill["tpa"]+200, lTwo), ratio(skill["tpm"]+300*lThree, skill["tpa"]+300, lThree), ratio(skill["ftm"]+100*lFree, skill["fta"]+100, lFree)}
}
