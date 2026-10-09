package roster

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/settings"
)

func roster(main, reserveProspects, reserveOthers int) []Entry {
	var entries []Entry
	for range main {
		entries = append(entries, Entry{List: settings.ListMain})
	}
	for range reserveProspects {
		entries = append(entries, Entry{List: settings.ListReserve, Prospect: true})
	}
	for range reserveOthers {
		entries = append(entries, Entry{List: settings.ListReserve})
	}
	return entries
}

func TestCheck(t *testing.T) {
	limits := settings.Roster{Main: 2, Reserve: 1, ReserveEligibility: settings.ReserveProspects}

	tests := []struct {
		name          string
		before, after []Entry
		allowed       bool
	}{
		{"add within limits", roster(1, 0, 0), roster(2, 0, 0), true},
		{"add beyond main limit", roster(2, 0, 0), roster(3, 0, 0), false},
		{"add prospect to reserve", roster(2, 0, 0), roster(2, 1, 0), true},
		{"add beyond reserve limit", roster(2, 1, 0), roster(2, 2, 0), false},
		{"non-prospect to reserve", roster(1, 0, 0), roster(1, 0, 1), false},
		{"over the limit, fixing it fully", roster(3, 0, 0), roster(2, 0, 0), true},
		{"over the limit, reducing it", roster(4, 0, 0), roster(3, 0, 0), true},
		{"over the limit, swapping one for one", roster(3, 0, 0), roster(3, 0, 0), false},
		{"over the limit, adding elsewhere", roster(3, 0, 0), roster(3, 1, 0), false},
		{"graduate on reserve moves to main", roster(1, 0, 1), roster(2, 0, 0), true},
	}
	for _, tt := range tests {
		err := Check(limits, tt.before, tt.after)
		if (err == nil) != tt.allowed {
			t.Errorf("%s: allowed = %v, want %v (err: %v)", tt.name, err == nil, tt.allowed, err)
		}
	}

	anyone := settings.Roster{Main: 2, Reserve: 1, ReserveEligibility: settings.ReserveAnyone}
	if err := Check(anyone, roster(1, 0, 0), roster(1, 0, 1)); err != nil {
		t.Errorf("reserve open to anyone refused a non-prospect: %v", err)
	}
}

func TestAcquirable(t *testing.T) {
	draft := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	held := pgtype.Timestamptz{Time: draft, Valid: true}
	veteran, rookie := draft.AddDate(-1, 0, 0), draft.AddDate(0, 1, 0)

	open := settings.FreeAgency{Mode: settings.FreeAgencyOpen, NewEntrantsDraftOnly: true}
	unguarded := settings.FreeAgency{Mode: settings.FreeAgencyOpen}
	closed := settings.FreeAgency{Mode: settings.FreeAgencyClosed}

	tests := []struct {
		name      string
		rules     settings.FreeAgency
		lastDraft pgtype.Timestamptz
		since     time.Time
		allowed   bool
	}{
		{"veteran after a draft", open, held, veteran, true},
		{"rookie who arrived after the draft", open, held, rookie, false},
		{"anyone before the first draft", open, pgtype.Timestamptz{}, veteran, false},
		{"rookie with the guard off", unguarded, held, rookie, true},
		{"before the first draft with the guard off", unguarded, pgtype.Timestamptz{}, veteran, true},
		{"free agency closed", closed, held, veteran, false},
	}
	for _, tt := range tests {
		err := Acquirable(tt.rules, tt.lastDraft, tt.since)
		if (err == nil) != tt.allowed {
			t.Errorf("%s: allowed = %v, want %v (err: %v)", tt.name, err == nil, tt.allowed, err)
		}
	}
}

func TestReserveOutsideStartingConferences(t *testing.T) {
	limits := settings.Roster{Main: 2, Reserve: 2, ReserveEligibility: settings.ReserveProspectsOrIneligible}
	for _, entry := range []Entry{{List: settings.ListReserve, Prospect: true}, {List: settings.ListReserve, StarterIneligible: true}} {
		if err := Check(limits, nil, []Entry{entry}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Check(limits, nil, []Entry{{List: settings.ListReserve}}); err == nil {
		t.Fatal("active starter-eligible player should require an anyone reserve rule")
	}
	limits.ReserveEligibility = settings.ReserveProspects
	if err := Check(limits, nil, []Entry{{List: settings.ListReserve, StarterIneligible: true}}); err == nil {
		t.Fatal("commissioner prospects-only rule ignored")
	}
}
