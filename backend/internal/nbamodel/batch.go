package nbamodel

import (
	"fmt"
	"time"
)

// PredictBatch shares one completed-prior-day state across prospective targets.
// Reports must have been published by observedAt; today's outcomes stay excluded.
func PredictBatch(rows []Row, model Model, targets []Target, reports map[string]*PregameReport, observedAt time.Time) ([]Prediction, []Features, error) {
	if err := model.Validate(); err != nil {
		return nil, nil, err
	}
	state := NewState()
	end := 0
	for end < len(rows) && sportsdayOf(rows[end].StartsAt).Before(sportsdayOf(observedAt)) {
		end++
	}
	if err := Walk(rows[:end], model.Config, func(_ Row, _ Features, s *State) error { state = s; return nil }); err != nil {
		return nil, nil, err
	}
	// Walk observes its final day after the visitor; state points at that same object.
	state.reports = map[string]*PregameReport{}
	predictions := make([]Prediction, len(targets))
	features := make([]Features, len(targets))
	for i, t := range targets {
		if !t.StartsAt.After(observedAt) || sportsdayOf(t.StartsAt).Before(model.TrainedBefore) {
			return nil, nil, fmt.Errorf("target %s is not prospective", t.GameID)
		}
		if r := reports[t.GameID]; r != nil {
			if r.PublishedAt.IsZero() || r.PublishedAt.After(observedAt) || !r.PublishedAt.Before(t.StartsAt) || r.StartsAt.Sub(t.StartsAt).Abs() > time.Minute {
				return nil, nil, fmt.Errorf("invalid report cutoff for %s", t.GameID)
			}
			for _, team := range r.Teams {
				if team.ObservedAt != nil && (team.ObservedAt.After(observedAt) || team.ObservedAt.IsZero() || !team.ObservedAt.Before(t.StartsAt)) {
					return nil, nil, fmt.Errorf("invalid team observation cutoff for %s", t.GameID)
				}
			}
			state.reports[t.GameID] = r
		}
		features[i] = state.Build(t, model.Config)
		predictions[i] = model.Forecast(t, features[i], state, model.Scoring)
	}
	return predictions, features, nil
}
