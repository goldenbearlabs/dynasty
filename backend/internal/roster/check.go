// Package roster owns every change to who is on which roster. All of them
// pass through Check, so roster limits are enforced in exactly one place.
package roster

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/problem"
	"crossover/internal/settings"
)

// Entry is what the limits need to know about one rostered player.
type Entry struct {
	PlayerID pgtype.UUID
	List     string
	Prospect bool
}

// Overage measures how far a roster is outside its limits: players beyond
// each list's size, plus players on the reserve list who may not be there.
// Zero means the roster is legal.
func Overage(limits settings.Roster, entries []Entry) int {
	main, reserve, ineligible := 0, 0, 0
	for _, e := range entries {
		switch {
		case e.List == settings.ListMain:
			main++
		case limits.ReserveEligibility == settings.ReserveProspects && !e.Prospect:
			reserve++
			ineligible++
		default:
			reserve++
		}
	}
	return max(0, main-limits.Main) + max(0, reserve-limits.Reserve) + ineligible
}

// Check decides whether a franchise may go from one roster to another. A
// legal result is always allowed. A franchise already over its limits (after
// a graduation, or a rule change) may only make moves that reduce the overage.
func Check(limits settings.Roster, before, after []Entry) error {
	was, now := Overage(limits, before), Overage(limits, after)
	switch {
	case now == 0, now < was:
		return nil
	case was > 0:
		return problem.New("This franchise is over its roster limits and can only make moves that fix that.")
	default:
		return explain(limits, after)
	}
}

// explain names the limit a roster breaks.
func explain(limits settings.Roster, entries []Entry) error {
	main, reserve := 0, 0
	for _, e := range entries {
		if e.List == settings.ListMain {
			main++
			continue
		}
		reserve++
		if limits.ReserveEligibility == settings.ReserveProspects && !e.Prospect {
			return problem.New("Only prospects can be on the reserve list in this league.")
		}
	}
	if main > limits.Main {
		return problem.New("The main roster is full (%d players).", limits.Main)
	}
	if reserve > limits.Reserve {
		return problem.New("The reserve list is full (%d players).", limits.Reserve)
	}
	return nil
}

// LockedUntil is when a player sent to the reserve list at reservedAt may
// return to the main roster. The zero time means he is not locked.
func LockedUntil(limits settings.Roster, reservedAt pgtype.Timestamptz) time.Time {
	if !reservedAt.Valid || limits.ReserveLockDays == 0 {
		return time.Time{}
	}
	return reservedAt.Time.AddDate(0, 0, limits.ReserveLockDays)
}

// Acquirable decides whether an unrostered player may be added as a free
// agent, outside a draft. lastDraft is when the league's last draft
// finished, if it has had one.
func Acquirable(rules settings.FreeAgency, lastDraft pgtype.Timestamptz, eligibleSince time.Time) error {
	if rules.Mode != settings.FreeAgencyOpen {
		return problem.New("Free agency is closed in this league.")
	}
	if !rules.NewEntrantsDraftOnly {
		return nil
	}
	if !lastDraft.Valid {
		return problem.New("Free agency opens after this league's first draft.")
	}
	if !eligibleSince.Before(lastDraft.Time) {
		return problem.New("This player joined the pool after the last draft and can only be acquired in the next one.")
	}
	return nil
}
