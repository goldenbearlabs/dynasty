package competition

import (
	"slices"
	"testing"

	"crossover/internal/ingest"
)

func TestEverySportHasRequestedPlayoffDefaults(t *testing.T) {
	registry := NewRegistry(ingest.NewClient(0))
	keys := []string{}
	for _, c := range registry {
		keys = append(keys, c.Key)
	}
	for _, c := range registry {
		t.Run(c.Key, func(t *testing.T) {
			f := c.Defaults.Format
			if f.PlayoffTeams != 8 || f.Reseed || !f.ConsolationEnabled() || !f.BottomFirst() || f.Ranking() != "record" || !slices.Equal(f.TieRules(), []string{"points_for", "head_to_head", "points_against"}) {
				t.Fatalf("defaults = %+v", f)
			}
			custom := c.Defaults
			disabled := false
			custom.Format.PlayoffTeams = 4
			custom.Format.Reseed = true
			custom.Format.Consolation = &disabled
			custom.Format.ConsolationSeeding = "top_first"
			custom.Format.StandingsBy = "win_percentage"
			custom.Format.Tiebreakers = []string{"points_against", "head_to_head", "points_for"}
			if err := custom.Validate(c.Catalog(keys)); err != nil {
				t.Fatalf("custom rules rejected: %v", err)
			}
			custom.Format.Tiebreakers = []string{"points_for", "points_for"}
			if err := custom.Validate(c.Catalog(keys)); err == nil {
				t.Fatal("duplicate tiebreaker accepted")
			}
			custom.Format.Tiebreakers = []string{"unknown"}
			if err := custom.Validate(c.Catalog(keys)); err == nil {
				t.Fatal("unknown tiebreaker accepted")
			}
		})
	}
}
