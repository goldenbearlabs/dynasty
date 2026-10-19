package scoring

import (
	"context"
	"slices"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"github.com/jackc/pgx/v5/pgtype"
)

const championshipBracket = "championship"
const consolationBracket = "consolation"

type bracketPair struct {
	Home, Away pgtype.UUID
	Slot       int32
}

func scheduleRounds(format settings.Format, teams int) int {
	top := min(format.PlayoffTeams, teams)
	rounds := playoffRounds(top)
	if format.ConsolationEnabled() {
		rounds = max(rounds, playoffRounds(teams-top))
	}
	return rounds
}

// The usual fixed bracket: 1–8, 4–5, 2–7, 3–6. Padding to the next
// power of two awards byes to the bracket's highest seeds. Consolation
// seeds default to reverse standings, so 14 and 13 receive the byes.
func seededBracket(teams []pgtype.UUID) []bracketPair {
	if len(teams) < 2 {
		return nil
	}
	order := []int{1, 2}
	for len(order) < len(teams) {
		next := make([]int, 0, len(order)*2)
		for _, seed := range order {
			next = append(next, seed, len(order)*2+1-seed)
		}
		order = next
	}
	side := func(seed int) pgtype.UUID {
		if seed > len(teams) {
			return pgtype.UUID{}
		}
		return teams[seed-1]
	}
	pairs := make([]bracketPair, 0, len(order)/2)
	for i := 0; i < len(order); i += 2 {
		home, away := side(order[i]), side(order[i+1])
		if !home.Valid {
			home, away = away, home
		}
		pairs = append(pairs, bracketPair{Home: home, Away: away, Slot: int32(i / 2)})
	}
	return pairs
}

func insertBracket(ctx context.Context, q *db.Queries, period pgtype.UUID, bracket string, pairs []bracketPair) error {
	for _, p := range pairs {
		if err := q.InsertBracketMatchup(ctx, db.InsertBracketMatchupParams{PeriodID: period, HomeFranchiseID: p.Home, AwayFranchiseID: p.Away, Bracket: bracket, BracketSlot: p.Slot}); err != nil {
			return err
		}
	}
	return nil
}

// pairCustom preserves a commissioner's bracket identities and slot order.
func pairCustom(ctx context.Context, q *db.Queries, period db.Period, input []Pairing, pairs [][2]pgtype.UUID) error {
	slots := map[string]int32{}
	used := map[string]map[int32]bool{}
	for i, p := range input {
		bracket := p.Bracket
		if bracket == "" {
			bracket = championshipBracket
		}
		if bracket != championshipBracket && bracket != consolationBracket {
			return problem.New("Choose a championship or consolation bracket.")
		}
		if !period.IsPlayoff && bracket == consolationBracket {
			return problem.New("Consolation matchups belong in the playoffs.")
		}
		slot := slots[bracket]
		if p.Slot != nil {
			slot = *p.Slot
		}
		if slot < 0 || slot > 63 {
			return problem.New("Bracket slots must be between 0 and 63.")
		}
		if used[bracket] == nil {
			used[bracket] = map[int32]bool{}
		}
		if used[bracket][slot] {
			return problem.New("Every matchup needs a unique slot in its bracket.")
		}
		used[bracket][slot] = true
		slots[bracket] = max(slots[bracket], slot+1)
		if err := insertBracket(ctx, q, period.ID, bracket, []bracketPair{{Home: pairs[i][0], Away: pairs[i][1], Slot: slot}}); err != nil {
			return err
		}
	}
	return nil
}

func fixedNext(previous []db.ListSeasonMatchupsRow, scored map[pgtype.UUID]Row) []bracketPair {
	if len(previous) < 2 {
		return nil
	}
	branches := map[int32]bracketPair{}
	for _, m := range previous {
		slot := m.BracketSlot / 2
		p := branches[slot]
		p.Slot = slot
		if m.BracketSlot%2 == 0 {
			p.Home = winner(m.HomeFranchiseID, m.AwayFranchiseID, scored)
		} else {
			p.Away = winner(m.HomeFranchiseID, m.AwayFranchiseID, scored)
		}
		branches[slot] = p
	}
	keys := make([]int32, 0, len(branches))
	for slot := range branches {
		keys = append(keys, slot)
	}
	slices.Sort(keys)
	pairs := make([]bracketPair, 0, len(keys))
	for _, slot := range keys {
		p := branches[slot]
		if !p.Home.Valid {
			p.Home, p.Away = p.Away, p.Home
		}
		pairs = append(pairs, p)
	}
	return pairs
}
