// Package nbamodel trains and evaluates pregame NBA projections. Outcomes are
// deliberately separate from Target: a game's minutes, starting status and box
// score cannot be supplied as prediction features.
package nbamodel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"time"

	"crossover/internal/sportsday"
)

const Version = 2

func sportsdayOf(t time.Time) time.Time { return sportsday.Of(t) }

var Stats = []string{"pts", "reb", "ast", "stl", "blk", "tov", "tpm", "fgm", "fga", "ftm", "fta", "dd", "td", "pts40", "pts50"}

type Row struct {
	GameID         string             `json:"game_id"`
	StartsAt       time.Time          `json:"starts_at"`
	Season         int                `json:"season"`
	PlayerID       string             `json:"player_id"`
	Name           string             `json:"name"`
	Team           string             `json:"team"`
	Opponent       string             `json:"opponent"`
	Position       string             `json:"position"`
	Home           bool               `json:"home"`
	Minutes        float64            `json:"minutes"`
	MinutesMissing bool               `json:"minutes_missing,omitempty"`
	Played         bool               `json:"played"`
	Starter        bool               `json:"starter"`
	Stats          map[string]float64 `json:"stats"`
	Pregame        *PregameReport     `json:"-"`
}

type Target struct {
	GameID   string    `json:"game_id"`
	StartsAt time.Time `json:"starts_at"`
	Season   int       `json:"season"`
	PlayerID string    `json:"player_id"`
	Team     string    `json:"team"`
	Opponent string    `json:"opponent"`
	Home     bool      `json:"home"`
}

func (r Row) Target() Target {
	return Target{r.GameID, r.StartsAt, r.Season, r.PlayerID, r.Team, r.Opponent, r.Home}
}

func ReadRows(reader io.Reader) ([]Row, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 65536), 2<<20)
	var rows []Row
	seen := map[string]bool{}
	for scanner.Scan() {
		var row Row
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("line %d: %w", len(rows)+1, err)
		}
		if row.GameID == "" || row.PlayerID == "" || row.Team == "" || row.Opponent == "" || row.Team == row.Opponent || row.StartsAt.IsZero() || row.Season < 2000 {
			return nil, fmt.Errorf("line %d: missing or invalid game/player context", len(rows)+1)
		}
		if math.IsNaN(row.Minutes) || math.IsInf(row.Minutes, 0) || row.Minutes < 0 || row.Minutes > 80 || ((!row.Played || row.MinutesMissing) && row.Minutes != 0) {
			return nil, fmt.Errorf("line %d: invalid minutes", len(rows)+1)
		}
		if row.Played {
			for _, stat := range Stats {
				v, ok := row.Stats[stat]
				if !ok || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
					return nil, fmt.Errorf("line %d: missing or invalid stat %s", len(rows)+1, stat)
				}
			}
		}
		key := row.GameID + "/" + row.PlayerID
		if seen[key] {
			return nil, fmt.Errorf("duplicate player-game %s", key)
		}
		seen[key] = true
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(rows, func(a, b Row) int {
		if c := a.StartsAt.Compare(b.StartsAt); c != 0 {
			return c
		}
		if a.GameID != b.GameID {
			if a.GameID < b.GameID {
				return -1
			}
			return 1
		}
		if a.PlayerID < b.PlayerID {
			return -1
		}
		if a.PlayerID > b.PlayerID {
			return 1
		}
		return 0
	})
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty NBA dataset")
	}
	return rows, nil
}

func Score(stats, scoring map[string]float64) float64 {
	var score float64
	for _, stat := range Stats {
		score += stats[stat] * scoring[stat]
	}
	return score
}

func Position(position string) string {
	switch position {
	case "PG", "SG", "G":
		return "G"
	case "SF", "PF", "F":
		return "F"
	case "C":
		return "C"
	default:
		return "UNK"
	}
}
