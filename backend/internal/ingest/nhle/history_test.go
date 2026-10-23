package nhle

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

func TestRecentSeasonIdentitiesIncludeSkatersAndGoaliesOnly(t *testing.T) {
	var bios int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/skater/summary":
			fmt.Fprint(w, `{"data":[{"playerId":8482093,"teamAbbrevs":"CAR","gamesPlayed":71,"goals":32}]}`)
		case "/skater/realtime":
			fmt.Fprint(w, `{"data":[]}`)
		case "/goalie/summary":
			fmt.Fprint(w, `{"data":[{"playerId":7,"teamAbbrevs":"CAR","gamesPlayed":9,"wins":6}]}`)
		case "/skater/bios":
			bios++
			fmt.Fprint(w, `{"data":[{"playerId":8482093,"skaterFullName":"Seth Jarvis","birthDate":"2002-02-01","positionCode":"R"}]}`)
		case "/goalie/bios":
			bios++
			fmt.Fprint(w, `{"data":[{"playerId":7,"goalieFullName":"Injured Goalie","birthDate":"1998-07-12"}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	src := New(ingest.NewClient(0))
	src.StatsBase = server.URL
	latest := src.LatestSeason(time.Now())
	for _, year := range []int{latest, latest - 1, latest - 2} {
		lines, err := src.SeasonStats(context.Background(), year)
		if err != nil || len(lines) != 2 {
			t.Fatalf("season lines=%+v err=%v", lines, err)
		}
		for _, line := range lines {
			if line.Player == nil || line.Player.Provider != "nhle" || line.Player.ProviderID != line.ProviderID || line.Player.BirthDate.IsZero() || !strings.Contains(line.Player.Note, "NHL history:") {
				t.Fatalf("missing identity: %+v", line)
			}
		}
		if lines[1].Player.Positions[0] != "G" {
			t.Fatal("goalie missing position")
		}
	}
	lines, err := src.SeasonStats(context.Background(), latest-3)
	if err != nil || bios != 6 || lines[0].Player != nil || lines[1].Player != nil {
		t.Fatalf("older seasons must not discover retired players: bios=%d err=%v", bios, err)
	}
}
