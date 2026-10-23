package scoring

import (
	"crossover/internal/db"
	"crossover/internal/settings"
	"github.com/jackc/pgx/v5/pgtype"
	"testing"
)

func TestFixedBracketAndConsolationByes(t *testing.T) {
	top := []pgtype.UUID{team(1), team(2), team(3), team(4), team(5), team(6), team(7), team(8)}
	for i, want := range [][2]byte{{1, 8}, {4, 5}, {2, 7}, {3, 6}} {
		p := seededBracket(top)[i]
		if p.Home != team(want[0]) || p.Away != team(want[1]) || p.Slot != int32(i) {
			t.Fatalf("slot %d = %+v", i, p)
		}
	}
	lower := []pgtype.UUID{team(14), team(13), team(12), team(11), team(10), team(9)}
	pairs := seededBracket(lower)
	if len(pairs) != 4 || pairs[0].Home != team(14) || pairs[0].Away.Valid || pairs[2].Home != team(13) || pairs[2].Away.Valid {
		t.Fatalf("consolation byes = %+v", pairs)
	}
	if pairs[1].Home != team(11) || pairs[1].Away != team(10) || pairs[3].Home != team(12) || pairs[3].Away != team(9) {
		t.Fatalf("consolation games = %+v", pairs)
	}
	previous := []db.ListSeasonMatchupsRow{}
	for _, p := range seededBracket(top) {
		previous = append(previous, db.ListSeasonMatchupsRow{HomeFranchiseID: p.Home, AwayFranchiseID: p.Away, BracketSlot: p.Slot})
	}
	// Seed 8 upsets 1. Its next opponent stays in the 4–5 branch, rather than seed 2.
	next := fixedNext(previous, map[pgtype.UUID]Row{team(8): {Points: 100}})
	if len(next) != 2 || next[0].Home != team(8) || next[0].Away != team(4) || next[1].Home != team(2) || next[1].Away != team(3) {
		t.Fatalf("fixed semifinals = %+v", next)
	}
}

func TestScheduleRoundsIncludeBothBrackets(t *testing.T) {
	format := settings.Format{PlayoffTeams: 8}
	if scheduleRounds(format, 14) != 3 {
		t.Fatal("8-team championship and 6-team consolation need three rounds")
	}
	format.PlayoffTeams = 2
	if scheduleRounds(format, 14) != 4 {
		t.Fatal("12-team consolation needs four rounds")
	}
	disabled := false
	format.Consolation = &disabled
	if scheduleRounds(format, 14) != 1 {
		t.Fatal("disabled consolation should not add rounds")
	}
}

func TestFixedBracketKeepsCustomPositions(t *testing.T) {
	previous := []db.ListSeasonMatchupsRow{
		{HomeFranchiseID: team(8), AwayFranchiseID: team(1), BracketSlot: 0},
		{HomeFranchiseID: team(2), AwayFranchiseID: team(7), BracketSlot: 2},
		{HomeFranchiseID: team(3), AwayFranchiseID: team(6), BracketSlot: 3},
	}
	next := fixedNext(previous, nil)
	if len(next) != 2 || next[0].Home != team(8) || next[0].Away.Valid || next[1].Home != team(2) || next[1].Away != team(3) {
		t.Fatalf("a missing slot changed the bracket path: %+v", next)
	}
}
