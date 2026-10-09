package competition

import (
	"testing"

	"crossover/internal/ingest"
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
