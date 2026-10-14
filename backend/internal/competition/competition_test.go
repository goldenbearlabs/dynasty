package competition

import (
	"testing"

	"crossover/internal/ingest"
	"crossover/internal/ingest/espn"
	"crossover/internal/ingest/mlbam"
)

// Every sport's default rules must pass the same validation a commissioner's
// edits do, so a new league never starts out invalid.
func TestDefaultsAreValid(t *testing.T) {
	registry := NewRegistry(ingest.NewClient(0))

	var keys []string
	for _, c := range registry {
		keys = append(keys, c.Key)
	}
	for _, c := range registry {
		if err := c.Defaults.Validate(c.Catalog(keys)); err != nil {
			t.Errorf("%s defaults: %v", c.Key, err)
		}
	}
}

func TestProfessionalProspectPools(t *testing.T) {
	registry := NewRegistry(ingest.NewClient(0))
	for _, key := range []string{"nfl", "wnba"} {
		c, _ := registry.Get(key)
		draft, ok := c.Prospects.(*espn.Draft)
		if !ok || !draft.PicksOnly || draft.Classes == 0 {
			t.Fatalf("%s must import completed picks rather than future college boards", key)
		}
	}
	c, _ := registry.Get("mlb")
	if _, ok := c.Prospects.(*mlbam.Source); !ok {
		t.Fatal("MLB must use its signed-pick and professional minor-league feed")
	}
	// NBA's existing college-to-pro pathway remains unchanged.
	c, _ = registry.Get("nba")
	if draft, ok := c.Prospects.(*espn.Draft); !ok || draft.PicksOnly {
		t.Fatal("NBA's existing prospect pathway changed")
	}
}
