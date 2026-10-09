package draft

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
)

// maxRanked keeps a ranking to a size someone could have made by hand.
const maxRanked = 500

// Ranking is a manager's pre-draft list for one league, with its players.
type Ranking struct {
	db.Ranking
	Players []db.ListRankingPlayersRow `json:"players"`
}

// RankingChange is what a manager may set on a ranking. LeagueID counts
// only when the ranking is created; PlayerIDs, when given, replace the list.
type RankingChange struct {
	Name      string         `json:"name"`
	LeagueID  pgtype.UUID    `json:"league_id"`
	DraftID   pgtype.UUID    `json:"draft_id"` // optional: the draft this list is for
	PlayerIDs *[]pgtype.UUID `json:"player_ids"`
}

func (s *Service) Rankings(ctx context.Context, franchise db.Franchise) ([]db.ListRankingsRow, error) {
	return db.New(s.pool).ListRankings(ctx, franchise.ID)
}

func (s *Service) Ranking(ctx context.Context, franchise db.Franchise, id pgtype.UUID) (Ranking, error) {
	return loadRanking(ctx, db.New(s.pool), franchise, id)
}

func loadRanking(ctx context.Context, q *db.Queries, franchise db.Franchise, id pgtype.UUID) (Ranking, error) {
	ranking, err := q.GetRanking(ctx, db.GetRankingParams{ID: id, FranchiseID: franchise.ID})
	if err != nil {
		return Ranking{}, err
	}
	players, err := q.ListRankingPlayers(ctx, id)
	if players == nil {
		players = []db.ListRankingPlayersRow{}
	}
	return Ranking{Ranking: ranking, Players: players}, err
}

// SaveRanking creates a ranking (when id is not set) or changes one.
func (s *Service) SaveRanking(ctx context.Context, franchise db.Franchise, id pgtype.UUID, in RankingChange) (Ranking, error) {
	var saved Ranking
	err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return problem.New("Give the ranking a name.")
		}
		var ranking db.Ranking
		var err error
		if id.Valid {
			if ranking, err = q.GetRanking(ctx, db.GetRankingParams{ID: id, FranchiseID: franchise.ID}); err != nil {
				return err
			}
		} else {
			league, err := q.GetLeague(ctx, in.LeagueID)
			if err != nil || league.DynastyID != franchise.DynastyID {
				return problem.New("Choose a league for the ranking.")
			}
			ranking.LeagueID = league.ID
		}
		if in.DraftID.Valid {
			covered, err := q.DraftCoversLeague(ctx, db.DraftCoversLeagueParams{DraftID: in.DraftID, LeagueID: ranking.LeagueID})
			if err != nil {
				return err
			}
			if !covered {
				return problem.New("That draft is not for this league.")
			}
		}
		if id.Valid {
			err = q.UpdateRanking(ctx, db.UpdateRankingParams{ID: id, Name: name, DraftID: in.DraftID})
		} else {
			ranking, err = q.CreateRanking(ctx, db.CreateRankingParams{
				FranchiseID: franchise.ID, LeagueID: ranking.LeagueID, DraftID: in.DraftID, Name: name,
			})
		}
		if err != nil {
			return err
		}

		if in.PlayerIDs != nil {
			if len(*in.PlayerIDs) > maxRanked {
				return problem.New("A ranking holds at most %d players.", maxRanked)
			}
			if err := q.ClearRankingPlayers(ctx, ranking.ID); err != nil {
				return err
			}
			for rank, playerID := range *in.PlayerIDs {
				n, err := q.InsertRankingPlayer(ctx, db.InsertRankingPlayerParams{RankingID: ranking.ID, PlayerID: playerID, Rank: int32(rank)})
				if err != nil || n == 0 {
					return problem.New("One of those players cannot be ranked here: the list is for one league, and a player appears once.")
				}
			}
		}
		saved, err = loadRanking(ctx, q, franchise, ranking.ID)
		return err
	})
	return saved, err
}

func (s *Service) DeleteRanking(ctx context.Context, franchise db.Franchise, id pgtype.UUID) error {
	n, err := db.New(s.pool).DeleteRanking(ctx, db.DeleteRankingParams{ID: id, FranchiseID: franchise.ID})
	if err == nil && n == 0 {
		return problem.New("That ranking no longer exists.")
	}
	return err
}

// ImportRanking adds a ranking's players to the end of the franchise's
// queue for a draft, in ranked order, leaving out anyone already on a
// roster or already queued. It reports how many were added.
func (s *Service) ImportRanking(ctx context.Context, draftID pgtype.UUID, franchise db.Franchise, rankingID pgtype.UUID) (int, error) {
	added := 0
	err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
		ranking, err := loadRanking(ctx, q, franchise, rankingID)
		if err != nil {
			return err
		}
		covered, err := q.DraftCoversLeague(ctx, db.DraftCoversLeagueParams{DraftID: draftID, LeagueID: ranking.LeagueID})
		if err != nil {
			return err
		}
		if !covered {
			return problem.New("That ranking is for a league this draft does not cover.")
		}
		queue, err := q.ListDraftQueue(ctx, db.ListDraftQueueParams{DraftID: draftID, FranchiseID: franchise.ID})
		if err != nil {
			return err
		}
		queued := map[pgtype.UUID]bool{}
		for _, p := range queue {
			queued[p.PlayerID] = true
		}
		rank := int32(0)
		if len(queue) > 0 {
			rank = queue[len(queue)-1].Rank + 1
		}
		for _, p := range ranking.Players {
			if p.OwnerName != "" || queued[p.PlayerID] {
				continue
			}
			if err := q.InsertDraftQueue(ctx, db.InsertDraftQueueParams{DraftID: draftID, FranchiseID: franchise.ID, PlayerID: p.PlayerID, Rank: rank}); err != nil {
				return err
			}
			rank++
			added++
		}
		return nil
	})
	return added, err
}
