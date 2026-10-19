package scoring

import (
	"cmp"
	"slices"

	"crossover/internal/settings"
	"github.com/jackc/pgx/v5/pgtype"
)

// rankStandings resolves tied groups one rule at a time. In particular,
// multi-team head-to-head uses a mini-table of just the tied teams, rather
// than a pairwise comparison that could produce a non-transitive sort.
func rankStandings(rows []Row, results []result, format settings.Format, names map[pgtype.UUID]string) {
	primary := func(a, b Row) int {
		if format.Type != settings.FormatHeadToHead {
			return cmp.Compare(b.Points, a.Points)
		}
		switch format.Ranking() {
		case "points":
			return cmp.Compare(b.PointsFor, a.PointsFor)
		case "win_percentage":
			return cmp.Compare(share(b), share(a))
		default:
			return cmp.Or(cmp.Compare(b.Wins, a.Wins), cmp.Compare(a.Losses, b.Losses), cmp.Compare(b.Ties, a.Ties))
		}
	}
	var resolve func([]Row, int)
	rules := format.TieRules()
	resolve = func(group []Row, rule int) {
		if len(group) < 2 {
			return
		}
		if rule >= len(rules) || format.Type != settings.FormatHeadToHead {
			slices.SortStableFunc(group, func(a, b Row) int {
				return cmp.Or(cmp.Compare(names[a.FranchiseID], names[b.FranchiseID]), cmp.Compare(a.FranchiseID.String(), b.FranchiseID.String()))
			})
			return
		}
		values := map[pgtype.UUID]float64{}
		switch rules[rule] {
		case "points_for":
			for _, r := range group {
				values[r.FranchiseID] = r.PointsFor
			}
		case "points_against":
			for _, r := range group {
				values[r.FranchiseID] = r.PointsAgainst
			}
		case "head_to_head":
			mini := map[pgtype.UUID]Row{}
			for _, r := range group {
				mini[r.FranchiseID] = Row{}
			}
			for _, m := range results {
				home, h := mini[m.Home]
				away, a := mini[m.Away]
				if !h || !a {
					continue
				}
				switch {
				case m.HomePoints > m.AwayPoints:
					home.Wins++
					away.Losses++
				case m.AwayPoints > m.HomePoints:
					away.Wins++
					home.Losses++
				default:
					home.Ties++
					away.Ties++
				}
				mini[m.Home], mini[m.Away] = home, away
			}
			for id, r := range mini {
				values[id] = share(r)
			}
		}
		slices.SortStableFunc(group, func(a, b Row) int { return cmp.Compare(values[b.FranchiseID], values[a.FranchiseID]) })
		for start := 0; start < len(group); {
			end := start + 1
			for end < len(group) && values[group[start].FranchiseID] == values[group[end].FranchiseID] {
				end++
			}
			resolve(group[start:end], rule+1)
			start = end
		}
	}
	slices.SortStableFunc(rows, primary)
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && primary(rows[start], rows[end]) == 0 {
			end++
		}
		resolve(rows[start:end], 0)
		start = end
	}
}
