// Package competition is the registry of supported sports. It is the only
// place that knows a sport's feed, positions, stats and default rules;
// everything else treats a competition as a key.
package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/settings"
)

type Conference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Competition struct {
	Conferences []Conference    `json:"conferences"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Positions   []string        `json:"positions"` // what a lineup slot may list
	Stats       []Stat          `json:"stats"`     // what scoring may reward
	Defaults    settings.League `json:"defaults"`  // starting rules for a new league

	// Season is when the real regular season usually runs, as month-day
	// pairs ("10-20" to "04-12"). It only prefills the form for a new
	// fantasy season; the commissioner sets the real dates.
	Season [2]string `json:"season"`

	Source    ingest.Source         `json:"-"`
	Games     ingest.GameSource     `json:"-"`
	Seasons   ingest.SeasonSource   `json:"-"`
	Career    ingest.CareerSource   `json:"-"` // nil when the season feed already has everything
	Prospects ingest.ProspectSource `json:"-"` // nil when new players arrive through rosters
}

// Stat is one countable thing a player does in a game.
type Stat struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Registry lists competitions in display order.
type Registry []Competition

func NewRegistry(client *ingest.Client) Registry {
	return Registry{cbb(client), nba(client), wnba(client), nhl(client), nfl(client), mlb(client)}
}

func (r Registry) Get(key string) (Competition, bool) {
	for _, c := range r {
		if c.Key == key {
			return c, true
		}
	}
	return Competition{}, false
}

// Catalog is what a league's settings are validated against.
func (c Competition) Catalog(otherLeagues []string) settings.Catalog {
	stats := make([]string, len(c.Stats))
	for i, s := range c.Stats {
		stats[i] = s.Key
	}
	conferences := make([]string, len(c.Conferences))
	for i, conference := range c.Conferences {
		conferences[i] = conference.ID
	}
	return settings.Catalog{Positions: c.Positions, Stats: stats, Conferences: conferences, OtherLeagues: otherLeagues}
}

// defaults are the rules every sport starts from; each sport then sets its
// own roster, lineup, scoring and draft length.
func defaults() settings.League {
	return settings.League{
		Roster: settings.Roster{ReserveEligibility: settings.ReserveProspects},
		Lineup: settings.Lineup{Period: settings.PeriodDay, WeekStart: "monday", Lock: settings.LockGameStart},
		// Head to head, a week at a time: the format the scoring was balanced for.
		Format: settings.Format{Type: settings.FormatHeadToHead, MatchupDays: 7, PlayoffTeams: 6},
		FreeAgency: settings.FreeAgency{
			Mode:                 settings.FreeAgencyOpen,
			NewEntrantsDraftOnly: true,
		},
		Waivers: settings.Waivers{Mode: settings.WaiversNone, Days: 2, Budget: 100},
		// Rounds 0: half the reserve list, rounded up. A week to sign the picks.
		Draft:  settings.Draft{Order: settings.OrderLinear, FutureYears: 3, SigningDays: 7},
		Trades: settings.Trades{Approval: settings.ApprovalNone},
	}
}

func slot(name string, count int, positions ...string) settings.Slot {
	return settings.Slot{Name: name, Positions: positions, Count: count}
}
