// nba-refresh publishes forecasts from the validated model, without refitting it.
package main

import (
	"crossover/internal/nbalive"
	"crossover/internal/nbamodel"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	modelPath := flag.String("model", "data/nba/model.json", "validated artifact")
	dataPath := flag.String("data", "data/nba/history-enriched.jsonl", "completed historical rows")
	targetsPath := flag.String("targets", "", "prospective targets JSON")
	reportsPath := flag.String("availability", "", "official report sidecar")
	output := flag.String("out", "data/nba/live-projections.json", "atomic forecast snapshot")
	flag.Parse()
	raw, err := os.ReadFile(*modelPath)
	if err != nil {
		return err
	}
	modelHash := sha256.Sum256(raw)
	var model nbamodel.Model
	if err = json.Unmarshal(raw, &model); err != nil {
		return err
	}
	if err = model.Validate(); err != nil {
		return err
	}
	file, err := os.Open(*dataPath)
	if err != nil {
		return err
	}
	rows, err := nbamodel.ReadRows(file)
	file.Close()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no history")
	}
	if model.Config.Features.Opportunities {
		for _, r := range rows {
			if r.Played && r.Minutes > 0 {
				if _, ok := r.Stats["tpa"]; !ok {
					return fmt.Errorf("missing three-point attempts")
				}
			}
		}
	}
	var request struct {
		ObservedAt time.Time         `json:"observed_at"`
		Targets    []nbamodel.Target `json:"targets"`
	}
	raw, err = os.ReadFile(*targetsPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	if request.ObservedAt.IsZero() || request.ObservedAt.After(now) || now.Sub(request.ObservedAt) > time.Hour {
		return fmt.Errorf("stale target capture")
	}
	historyThrough := rows[len(rows)-1].StartsAt
	if now.Sub(historyThrough) > 96*time.Hour {
		return fmt.Errorf("history is older than 96 hours")
	}
	reports := map[string]*nbamodel.PregameReport{}
	if *reportsPath != "" {
		f, e := os.Open(*reportsPath)
		if e != nil {
			return e
		}
		reports, err = nbamodel.ReadPregameReports(f, rows)
		f.Close()
		if err != nil {
			return err
		}
	}
	predictions, features, err := nbamodel.PredictBatch(rows, model, request.Targets, reports, request.ObservedAt)
	if err != nil {
		return err
	}
	// A game that starts while the batch is computing must not receive a
	// newly published forecast claiming it was available before tipoff.
	now = time.Now().UTC()
	hash := modelHash
	snapshot := nbalive.Snapshot{Version: nbalive.Version, GeneratedAt: now, HistoryThrough: historyThrough, ModelSHA256: hex.EncodeToString(hash[:]), Records: map[string]nbalive.Record{}}
	// Preserve pre-tip forecasts for games that have started. Never replace them
	// with forecasts incorporating their own box scores or later injury reports.
	if prior, err := os.ReadFile(*output); err == nil {
		var old nbalive.Snapshot
		if err = json.Unmarshal(prior, &old); err != nil {
			return err
		}
		if err = old.Validate(now); err != nil {
			return err
		}
		for key, r := range old.Records {
			if !r.Target.StartsAt.After(now) && now.Sub(r.Target.StartsAt) < 30*24*time.Hour {
				snapshot.Records[key] = r
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for i, t := range request.Targets {
		if !t.StartsAt.After(now) || features[i].HistoryGames == 0 {
			continue
		}
		official := features[i].Official
		out := official != nil && official.Available && official.PlayerStatus == "Out"
		snapshot.Records[nbalive.Key(t.GameID, t.PlayerID)] = nbalive.Record{Target: t, Prediction: predictions[i], ForecastAt: request.ObservedAt, Out: out, Official: official}
	}
	if err = snapshot.Validate(now); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(*output), ".nba-projections-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(encoded); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0644); err != nil {
		return err
	}
	if err = os.Rename(name, *output); err != nil {
		return err
	}
	fmt.Printf("Published %d forecasts; history through %s; model %s\n", len(snapshot.Records), historyThrough.Format(time.RFC3339), snapshot.ModelSHA256)
	return nil
}
