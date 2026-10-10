// Package settings defines every rule a commissioner can change. A league's
// rules are one League value stored as JSON; adding a rule means adding a
// field here, a default in the competition registry and a check in Validate.
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"crossover/internal/sportsday"
)

// Dynasty holds the rules that span leagues.
type Dynasty struct {
	OverallTitle OverallTitle `json:"overall_title"`
}

// OverallTitle awards points for each league finish and sums them.
type OverallTitle struct {
	Enabled        bool      `json:"enabled"`
	PointsByFinish []float64 `json:"points_by_finish"` // first place first
}

// League holds one league's rules.
type League struct {
	Roster     Roster             `json:"roster"`
	Lineup     Lineup             `json:"lineup"`
	Scoring    map[string]float64 `json:"scoring"` // points per stat key
	Format     Format             `json:"format"`
	FreeAgency FreeAgency         `json:"free_agency"`
	Waivers    Waivers            `json:"waivers"`
	Draft      Draft              `json:"draft"`
	Trades     Trades             `json:"trades"`
	Continuity *Continuity        `json:"continuity"` // nil when players do not carry over
}

type Roster struct {
	Main               int    `json:"main"`
	Reserve            int    `json:"reserve"`
	ReserveEligibility string `json:"reserve_eligibility"` // ReserveProspects | ReserveAnyone
	// How long a player who is not a prospect must stay on the reserve list
	// once his manager sends him there. 0 means he can return at any time.
	ReserveLockDays int `json:"reserve_lock_days"`
	// When true, nobody on the reserve list can be called up to the main
	// roster while the league's season is being played, only between seasons.
	ReserveLockSeason bool `json:"reserve_lock_season"`
}

type Lineup struct {
	// Conferences limits starters, not the draft or reserve pool. Empty allows all.
	Conferences []string `json:"conferences"`
	Period      string   `json:"period"`     // PeriodDay | PeriodWeek
	WeekStart   string   `json:"week_start"` // weekday a weekly lineup begins on, e.g. "monday"
	Lock        string   `json:"lock"`       // LockGameStart | LockPeriodStart
	Slots       []Slot   `json:"slots"`
	// PitcherStartsPerWeek, when above zero, is how many starts by a team's
	// pitchers score in a week; a start beyond it scores nothing. A reliever
	// who opens a game counts only if he pitches more than an inning. Baseball only.
	PitcherStartsPerWeek int `json:"pitcher_starts_per_week"`
}

// Slot is a starting position. A player fits when one of his positions is
// listed, or the list contains AnyPosition.
type Slot struct {
	Name      string   `json:"name"`
	Positions []string `json:"positions"`
	Count     int      `json:"count"`
	// GamesPerWeek, when above zero, is how many of his games in a week
	// count for a player in this slot: one game a week for a basketball
	// player, one start a week for a starting pitcher. The manager picks
	// the game; left alone, it is his first. It needs weekly lineups.
	GamesPerWeek int `json:"games_per_week"`
}

type Format struct {
	Type         string `json:"type"`          // FormatTotalPoints | FormatHeadToHead
	MatchupDays  int    `json:"matchup_days"`  // head-to-head only
	PlayoffTeams int    `json:"playoff_teams"` // head-to-head only
}

type FreeAgency struct {
	Mode string `json:"mode"` // FreeAgencyOpen | FreeAgencyClosed
	// When true, a player who entered the pool after the league's last draft
	// can only be acquired in the next draft.
	NewEntrantsDraftOnly bool `json:"new_entrants_draft_only"`
	// How many players a franchise may acquire in a week, as free agents or
	// off waivers. 0 means no limit. The week starts on Lineup.WeekStart.
	WeeklyLimit int `json:"weekly_limit"`
}

// Waivers decides what happens to a dropped player. With waivers on he
// cannot be added for Days; franchises put in claims, and when the time is
// up the best claim gets him. Unclaimed, he becomes a free agent.
type Waivers struct {
	Mode   string `json:"mode"`   // WaiversNone | WaiversRolling | WaiversFAAB
	Hours  int    `json:"hours"`  // how long a released player stays on waivers
	Budget int    `json:"budget"` // FAAB only: what each franchise can bid in a season
}

// Draft holds the rules of a league's yearly rookie draft.
type Draft struct {
	// Rounds in a rookie draft. 0 means half the reserve list, rounded up,
	// so the draft class is sized to the list it has to be signed to.
	Rounds           int    `json:"rounds"`
	Order            string `json:"order"`              // OrderLinear | OrderSnake
	PickClockSeconds int    `json:"pick_clock_seconds"` // 0 means untimed
	FutureYears      int    `json:"future_years"`       // how far ahead picks can be traded
	// SigningDays is how long after a rookie draft a franchise has to sign
	// its picks to the reserve list before they are released.
	SigningDays int `json:"signing_days"`
}

// RookieRounds is how many rounds the league's rookie draft has.
func (l League) RookieRounds() int {
	if l.Draft.Rounds > 0 {
		return l.Draft.Rounds
	}
	return max(1, (l.Roster.Reserve+1)/2)
}

type Trades struct {
	Deadline string `json:"deadline"` // YYYY-MM-DD, or empty for none
	Approval string `json:"approval"` // ApprovalNone | ApprovalCommissioner
}

// Continuity carries this league's players into another league under the
// same franchise when they move up, as college players do into the NBA.
type Continuity struct {
	Into   string `json:"into"`    // competition key of the destination league
	LandOn string `json:"land_on"` // "main" | "reserve"
}

const (
	ReserveProspects             = "prospects_only"
	ReserveAnyone                = "anyone"
	ReserveProspectsOrIneligible = "prospects_or_ineligible"

	PeriodDay  = "day"
	PeriodWeek = "week"

	LockGameStart   = "game_start"
	LockPeriodStart = "period_start"

	FormatTotalPoints = "total_points"
	FormatHeadToHead  = "head_to_head"

	FreeAgencyOpen   = "open"
	FreeAgencyClosed = "closed"

	// WaiversRolling awards a player to the claim highest in the waiver
	// order, and the winner goes to the back of it. WaiversFAAB awards him
	// to the highest blind bid, with the waiver order breaking ties.
	WaiversNone    = "none"
	WaiversRolling = "rolling"
	WaiversFAAB    = "faab"

	OrderLinear = "linear"
	OrderSnake  = "snake"

	ApprovalNone         = "none"
	ApprovalCommissioner = "commissioner"

	ListMain    = "main"
	ListReserve = "reserve"
	// ListRights holds rookie-draft picks not yet signed: owned, but on
	// neither list and counted against no limit.
	ListRights = "rights"

	AnyPosition = "*"
)

// Catalog is what a league's settings are validated against.
type Catalog struct {
	Positions    []string // valid positions for the competition
	Stats        []string // valid scoring stat keys
	Conferences  []string // valid conference IDs; empty for professional sports
	OtherLeagues []string // competitions of the dynasty's other leagues
}

// Parse decodes stored or submitted settings, rejecting unknown fields so a
// misspelled rule is an error and not a silently ignored one.
func Parse[T Dynasty | League](raw []byte) (T, error) {
	var v T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&v)
	return v, err
}

func (d Dynasty) Validate() error {
	if slices.ContainsFunc(d.OverallTitle.PointsByFinish, func(p float64) bool { return p < 0 }) {
		return fmt.Errorf("overall_title.points_by_finish cannot contain negative points")
	}
	return nil
}

func (l League) Validate(c Catalog) error {
	checks := []error{
		between("roster.main", l.Roster.Main, 1, 100),
		between("roster.reserve", l.Roster.Reserve, 0, 100),
		oneOf("roster.reserve_eligibility", l.Roster.ReserveEligibility, ReserveProspects, ReserveAnyone, ReserveProspectsOrIneligible),
		between("roster.reserve_lock_days", l.Roster.ReserveLockDays, 0, 365),
		oneOf("lineup.period", l.Lineup.Period, PeriodDay, PeriodWeek),
		oneOf("lineup.week_start", l.Lineup.WeekStart, sportsday.Weekdays...),
		oneOf("lineup.lock", l.Lineup.Lock, LockGameStart, LockPeriodStart),
		between("lineup.pitcher_starts_per_week", l.Lineup.PitcherStartsPerWeek, 0, 30),
		l.validateSlots(c),
		l.validateConferences(c),
		l.validateScoring(c),
		oneOf("format.type", l.Format.Type, FormatTotalPoints, FormatHeadToHead),
		oneOf("free_agency.mode", l.FreeAgency.Mode, FreeAgencyOpen, FreeAgencyClosed),
		between("free_agency.weekly_limit", l.FreeAgency.WeeklyLimit, 0, 100),
		oneOf("waivers.mode", l.Waivers.Mode, WaiversNone, WaiversRolling, WaiversFAAB),
		between("waivers.hours", l.Waivers.Hours, 1, 14*24),
		between("waivers.budget", l.Waivers.Budget, 0, 100000),
		between("draft.rounds", l.Draft.Rounds, 0, 100),
		between("draft.signing_days", l.Draft.SigningDays, 1, 60),
		oneOf("draft.order", l.Draft.Order, OrderLinear, OrderSnake),
		between("draft.pick_clock_seconds", l.Draft.PickClockSeconds, 0, 7*24*3600),
		between("draft.future_years", l.Draft.FutureYears, 0, 10),
		oneOf("trades.approval", l.Trades.Approval, ApprovalNone, ApprovalCommissioner),
		l.validateContinuity(c),
	}
	if l.Format.Type == FormatHeadToHead {
		checks = append(checks,
			between("format.matchup_days", l.Format.MatchupDays, 1, 31),
			between("format.playoff_teams", l.Format.PlayoffTeams, 2, 64),
		)
	}
	if l.Trades.Deadline != "" {
		if _, err := time.Parse(time.DateOnly, l.Trades.Deadline); err != nil {
			checks = append(checks, fmt.Errorf("trades.deadline must be a date like 2027-02-15"))
		}
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

func (l League) validateConferences(c Catalog) error {
	seen := map[string]bool{}
	for _, id := range l.Lineup.Conferences {
		if seen[id] || !slices.Contains(c.Conferences, id) {
			return fmt.Errorf("lineup.conferences lists duplicate or unknown conference %q", id)
		}
		seen[id] = true
	}
	return nil
}

// StarterConferences supplies defaults for legacy CBB settings.
func (l League) StarterConferences(competition string) []string {
	if competition == "cbb" && l.Lineup.Conferences == nil {
		return DefaultCollegeConferences()
	}
	return l.Lineup.Conferences
}

// CanStart also rejects unknown membership when conferences are restricted.
func (l League) CanStart(competition, conference string) bool {
	ids := l.StarterConferences(competition)
	return len(ids) == 0 || slices.Contains(ids, conference)
}

func DefaultCollegeConferences() []string {
	return []string{"8", "23", "7", "2", "4", "44", "3", "21"}
}

func (l League) validateSlots(c Catalog) error {
	starters := 0
	names := map[string]bool{}
	for _, slot := range l.Lineup.Slots {
		if slot.Name == "" || names[slot.Name] {
			return fmt.Errorf("lineup.slots need unique, non-empty names")
		}
		names[slot.Name] = true
		if slot.Count < 1 {
			return fmt.Errorf("lineup slot %s needs a count of at least 1", slot.Name)
		}
		if len(slot.Positions) == 0 {
			return fmt.Errorf("lineup slot %s needs at least one position", slot.Name)
		}
		for _, position := range slot.Positions {
			if position != AnyPosition && !slices.Contains(c.Positions, position) {
				return fmt.Errorf("lineup slot %s lists unknown position %q", slot.Name, position)
			}
		}
		if slot.GamesPerWeek < 0 || slot.GamesPerWeek > 7 {
			return fmt.Errorf("lineup slot %s can count between 0 (all) and 7 games a week", slot.Name)
		}
		if slot.GamesPerWeek > 0 && l.Lineup.Period != PeriodWeek {
			return fmt.Errorf("lineup slot %s counts %d games a week, which needs weekly lineups", slot.Name, slot.GamesPerWeek)
		}
		starters += slot.Count
	}
	if starters > l.Roster.Main {
		return fmt.Errorf("lineup has %d starting slots but the main roster holds only %d", starters, l.Roster.Main)
	}
	return nil
}

func (l League) validateScoring(c Catalog) error {
	for stat := range l.Scoring {
		if !slices.Contains(c.Stats, stat) {
			return fmt.Errorf("scoring lists unknown stat %q", stat)
		}
	}
	return nil
}

func (l League) validateContinuity(c Catalog) error {
	if l.Continuity == nil {
		return nil
	}
	if !slices.Contains(c.OtherLeagues, l.Continuity.Into) {
		return fmt.Errorf("continuity.into must be another league in this dynasty, got %q", l.Continuity.Into)
	}
	return oneOf("continuity.land_on", l.Continuity.LandOn, ListMain, ListReserve)
}

func between(field string, value, low, high int) error {
	if value < low || value > high {
		return fmt.Errorf("%s must be between %d and %d", field, low, high)
	}
	return nil
}

func oneOf(field, value string, allowed ...string) error {
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s must be one of %v", field, allowed)
	}
	return nil
}
