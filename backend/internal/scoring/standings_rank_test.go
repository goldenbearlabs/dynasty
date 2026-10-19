package scoring

import (
	"crossover/internal/settings"
	"github.com/jackc/pgx/v5/pgtype"
	"testing"
)

func TestStandingsTieOrder(t *testing.T) {
	format := settings.Format{Type: settings.FormatHeadToHead}
	tests := []struct {
		name    string
		rows    []Row
		results []result
		first   byte
	}{
		{"wins before points", []Row{{FranchiseID: team(1), Wins: 2, PointsFor: 10}, {FranchiseID: team(2), Wins: 1, PointsFor: 1000}}, nil, 1},
		{"fewer losses", []Row{{FranchiseID: team(1), Wins: 2, Losses: 2, PointsFor: 1000}, {FranchiseID: team(2), Wins: 2, Losses: 1}}, nil, 2},
		{"points for before head to head", []Row{{FranchiseID: team(1), Wins: 2, PointsFor: 100}, {FranchiseID: team(2), Wins: 2, PointsFor: 90}}, []result{{Home: team(2), Away: team(1), HomePoints: 50, AwayPoints: 40}}, 1},
		{"head to head before points against", []Row{{FranchiseID: team(1), Wins: 2, PointsFor: 100, PointsAgainst: 1000}, {FranchiseID: team(2), Wins: 2, PointsFor: 100}}, []result{{Home: team(2), Away: team(1), HomePoints: 50, AwayPoints: 40}}, 2},
		{"higher points against wins", []Row{{FranchiseID: team(1), Wins: 2, PointsFor: 100, PointsAgainst: 90}, {FranchiseID: team(2), Wins: 2, PointsFor: 100, PointsAgainst: 110}}, nil, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rankStandings(tt.rows, tt.results, format, map[pgtype.UUID]string{})
			if tt.rows[0].FranchiseID != team(tt.first) {
				t.Fatalf("order = %+v", tt.rows)
			}
		})
	}
}

func TestHeadToHeadMultiTeamTieAndCustomOrder(t *testing.T) {
	rows := []Row{{FranchiseID: team(1), Wins: 5, PointsFor: 100, PointsAgainst: 10}, {FranchiseID: team(2), Wins: 5, PointsFor: 100, PointsAgainst: 20}, {FranchiseID: team(3), Wins: 5, PointsFor: 100, PointsAgainst: 30}}
	results := []result{{Home: team(1), Away: team(2), HomePoints: 10, AwayPoints: 0}, {Home: team(2), Away: team(3), HomePoints: 10, AwayPoints: 0}, {Home: team(3), Away: team(1), HomePoints: 10, AwayPoints: 0}}
	format := settings.Format{Type: settings.FormatHeadToHead}
	rankStandings(rows, results, format, nil)
	if rows[0].FranchiseID != team(3) || rows[1].FranchiseID != team(2) {
		t.Fatalf("circular head-to-head should fall through to higher PA: %+v", rows)
	}
	format.Tiebreakers = []string{"points_against", "points_for"}
	rows[0].PointsFor = 1
	rankStandings(rows, results, format, nil)
	if rows[0].FranchiseID != team(3) {
		t.Fatal("custom tiebreaker priority ignored")
	}
}
