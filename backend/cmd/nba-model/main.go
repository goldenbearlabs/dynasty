// nba-model is an offline, reproducible NBA projection laboratory. It never
// changes production projections or writes to the application database.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"crossover/internal/competition"
	"crossover/internal/nbamodel"
	"crossover/internal/sportsday"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: nba-model <inspect|train|evaluate|predict> [flags]")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	data := flags.String("data", "../data/nba/history.jsonl", "historical player-game JSONL")
	modelPath := flags.String("model", "../data/nba/model.json", "model artifact (train writes; evaluate/predict read)")
	reportPath := flags.String("report", "", "optional JSON report output")
	predictionPath := flags.String("predictions", "", "optional per-player-game JSONL output")
	errorPath := flags.String("errors", "", "optional compact player-game predictions for paired comparisons")
	refitBefore := flags.String("refit-before", "", "evaluate: refit the saved configuration strictly before this date, for chronological calibration")
	diagnostics := flags.Bool("diagnostics", false, "evaluate: include clearly labelled non-predictive oracle-minute and bonus diagnostics in compact errors")
	freezeWeek := flags.Bool("freeze-week", false, "evaluate: freeze player form/rest/workload at the week's first eligible day to test future matchup ordering")
	rankFeatures := flags.Bool("rank-features", false, "evaluate: export pregame matchup features for a separate ranking model")
	trainBefore := flags.String("train-before", "", "first validation sports day, YYYY-MM-DD")
	from := flags.String("test-from", "", "first held-out test sports day, YYYY-MM-DD")
	until := flags.String("test-to", "", "exclusive end sports day, YYYY-MM-DD (default: after last dataset game)")
	games := flags.String("games", "", "comma-separated ESPN game IDs; evaluation filter only")
	gameFile := flags.String("game-ids-file", "", "newline-separated ESPN game IDs; evaluation filter only")
	players := flags.String("players", "", "comma-separated ESPN player IDs; evaluation filter only")
	scoringFile := flags.String("scoring", "", "optional JSON map of canonical stat weights")
	targetFile := flags.String("target", "", "predict: pregame Target JSON file (no outcomes)")
	skipEvaluation := flags.Bool("skip-evaluation", false, "train: save a fitted artifact and validation results without evaluating a test window")
	featureNames := flags.String("features", "", "train: comma-separated groups including workload,role_state,stat_windows,opportunities,reported_availability,teammate_minutes,teammate_rates,team_budget,offseason,pace,competitiveness")
	availabilityPath := flags.String("availability", "", "optional timestamped official pregame-injury JSON sidecar")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if command != "inspect" && command != "train" && command != "evaluate" && command != "predict" {
		return fmt.Errorf("unknown command %q", command)
	}
	if *skipEvaluation && command != "train" {
		return fmt.Errorf("--skip-evaluation is only valid for train")
	}
	if *featureNames != "" && command != "train" {
		return fmt.Errorf("--features is a training option; evaluation and prediction use the artifact's saved feature set")
	}
	if (*refitBefore != "" || *diagnostics || *freezeWeek) && command != "evaluate" {
		return fmt.Errorf("--refit-before and --diagnostics are evaluation options")
	}
	// The enrichment preserves every original outcome and adds offensive
	// rebounds needed by pace models. Explicit --data always takes precedence.
	explicitData := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "data" {
			explicitData = true
		}
	})
	if !explicitData {
		if _, err := os.Stat("../data/nba/history-enriched.jsonl"); err == nil {
			*data = "../data/nba/history-enriched.jsonl"
		}
	}
	file, err := os.Open(*data)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	rows, err := nbamodel.ReadRows(io.TeeReader(file, hash))
	if err != nil {
		return err
	}
	var availability map[string]*nbamodel.PregameReport
	availabilityDigest := ""
	loadAvailability := func(path string) error {
		reports, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		availability, err = nbamodel.ReadPregameReports(io.TeeReader(reports, h), rows)
		availabilityDigest = hex.EncodeToString(h.Sum(nil))
		reports.Close()
		if err != nil {
			return err
		}
		return nil
	}
	if *availabilityPath != "" {
		if err := loadAvailability(*availabilityPath); err != nil {
			return err
		}
	}
	type coverage struct {
		Rows           int         `json:"rows"`
		Appearances    int         `json:"positive_minute_appearances"`
		DNP            int         `json:"dnp_or_zero_minutes"`
		MissingMinutes int         `json:"missing_minutes"`
		Games          int         `json:"games"`
		First          string      `json:"first_sports_day"`
		Last           string      `json:"last_sports_day"`
		Seasons        map[int]int `json:"rows_by_season"`
	}
	cov := coverage{Rows: len(rows), First: sportsday.Of(rows[0].StartsAt).Format(time.DateOnly), Last: sportsday.Of(rows[len(rows)-1].StartsAt).Format(time.DateOnly), Seasons: map[int]int{}}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.GameID] = true
		cov.Seasons[row.Season]++
		if row.MinutesMissing {
			cov.MissingMinutes++
		}
		if row.Played && row.Minutes > 0 {
			cov.Appearances++
		} else {
			cov.DNP++
		}
	}
	cov.Games = len(seen)
	digest := hex.EncodeToString(hash.Sum(nil))
	if command == "inspect" {
		return emit(*reportPath, map[string]any{"coverage": cov, "sha256": digest})
	}
	nba, _ := competition.NewRegistry(nil).Get("nba")
	scoring := nba.Defaults.Scoring
	if *scoringFile != "" {
		scoring = map[string]float64{}
		if err := readJSON(*scoringFile, &scoring); err != nil {
			return err
		}
	}
	knownStats := map[string]bool{}
	for _, stat := range nbamodel.Stats {
		knownStats[stat] = true
	}
	for stat := range scoring {
		if !knownStats[stat] {
			return fmt.Errorf("unsupported scoring stat %q", stat)
		}
	}
	if len(scoring) == 0 {
		return fmt.Errorf("scoring must not be empty")
	}
	end := sportsday.Of(rows[len(rows)-1].StartsAt).AddDate(0, 0, 1)
	if *until != "" {
		end, err = time.Parse(time.DateOnly, *until)
		if err != nil {
			return fmt.Errorf("test-to: %w", err)
		}
	}
	var model nbamodel.Model
	var candidates []nbamodel.Candidate
	var start time.Time
	if command == "train" {
		boundary, err := time.Parse(time.DateOnly, *trainBefore)
		if err != nil {
			return fmt.Errorf("train requires --train-before YYYY-MM-DD: %w", err)
		}
		start, err = time.Parse(time.DateOnly, *from)
		if err != nil {
			return fmt.Errorf("train requires --test-from YYYY-MM-DD: %w", err)
		}
		featureSet, err := nbamodel.ParseFeatureSet(*featureNames)
		if err != nil {
			return err
		}
		if featureSet.Pace {
			for _, row := range rows {
				if row.Played && row.Minutes > 0 {
					if _, ok := row.Stats["oreb"]; !ok {
						return fmt.Errorf("pace requires enriched data with offensive rebounds; missing for %s/%s", row.GameID, row.PlayerID)
					}
				}
			}
		}
		if featureSet.ReportedAvailability && len(availability) == 0 {
			return fmt.Errorf("reported_availability requires --availability with timestamped reports")
		}
		if featureSet.Opportunities {
			for _, r := range rows {
				if r.Played && r.Minutes > 0 {
					if _, ok := r.Stats["tpa"]; !ok {
						return fmt.Errorf("opportunities requires three-point attempts in the dataset")
					}
				}
			}
		}
		model, candidates, err = nbamodel.TrainVariant(rows, boundary, start, scoring, featureSet, func(s string) { fmt.Fprintln(os.Stderr, s) })
		if err != nil {
			return err
		}
	} else {
		if err := readJSON(*modelPath, &model); err != nil {
			return err
		}
		if err := model.Validate(); err != nil {
			return err
		}
		if model.Config.Features.ReportedAvailability && len(availability) == 0 && *availabilityPath == "" {
			*availabilityPath = "../data/nba/pregame-injuries.json"
			if err := loadAvailability(*availabilityPath); err != nil {
				return fmt.Errorf("model needs official report sidecar (--availability): %w", err)
			}
		}
		if model.Config.Features.ReportedAvailability && len(availability) == 0 {
			return fmt.Errorf("this model requires --availability with timestamped reports")
		}
		if *refitBefore != "" {
			before, err := time.Parse(time.DateOnly, *refitBefore)
			if err != nil {
				return err
			}
			model, err = nbamodel.FitBefore(rows, model.Config, before, scoring)
			if err != nil {
				return err
			}
		}
		if model.Config.Features.Pace {
			for _, row := range rows {
				if row.Played && row.Minutes > 0 {
					if _, ok := row.Stats["oreb"]; !ok {
						return fmt.Errorf("pace model requires enriched data with offensive rebounds; missing for %s/%s", row.GameID, row.PlayerID)
					}
				}
			}
		}
		if model.Config.Features.Opportunities {
			for _, r := range rows {
				if r.Played && r.Minutes > 0 {
					if _, ok := r.Stats["tpa"]; !ok {
						return fmt.Errorf("opportunity model requires three-point attempts in the dataset")
					}
				}
			}
		}
		start = model.TrainedBefore
		if *from != "" {
			start, err = time.Parse(time.DateOnly, *from)
			if err != nil {
				return fmt.Errorf("test-from: %w", err)
			}
		}
	}
	if command == "predict" {
		var target nbamodel.Target
		if err := readJSON(*targetFile, &target); err != nil {
			return err
		}
		if target.PlayerID == "" || target.Team == "" || target.Opponent == "" || target.Team == target.Opponent || target.StartsAt.IsZero() || target.Season < 2000 {
			return fmt.Errorf("target requires player_id, team, opponent, starts_at and season")
		}
		prediction, features, err := nbamodel.PredictWithReport(rows, model, target, scoring, availability[target.GameID])
		if err != nil {
			return err
		}
		return emit(*reportPath, map[string]any{"target": target, "prediction": prediction, "features": features})
	}
	if *skipEvaluation {
		if err := writeJSON(*modelPath, model); err != nil {
			return err
		}
		return emit(*reportPath, map[string]any{"dataset": *data, "dataset_sha256": digest, "availability_sha256": availabilityDigest, "availability_reports": len(availability), "coverage": cov, "model_training_examples": model.TrainingExamples, "model_trained_before": model.TrainedBefore, "selected_config": model.Config, "scoring": scoring, "validation_candidates": candidates})
	}
	gameIDs := splitIDs(*games)
	if *gameFile != "" {
		body, err := os.ReadFile(*gameFile)
		if err != nil {
			return err
		}
		for _, id := range strings.Fields(string(body)) {
			gameIDs[id] = true
		}
	}
	options := nbamodel.EvaluationOptions{From: start, To: end, GameIDs: gameIDs, PlayerIDs: splitIDs(*players), Diagnostics: *diagnostics, FreezeWeek: *freezeWeek, RankFeatures: *rankFeatures}
	var predictionFile *os.File
	var temporary string
	if *predictionPath != "" {
		if err := os.MkdirAll(filepath.Dir(*predictionPath), 0755); err != nil {
			return err
		}
		temporary = *predictionPath + ".tmp"
		predictionFile, err = os.Create(temporary)
		if err != nil {
			return err
		}
		defer func() { predictionFile.Close(); os.Remove(temporary) }()
		options.Predictions = predictionFile
	}
	var errorFile *os.File
	if *errorPath != "" {
		if err := os.MkdirAll(filepath.Dir(*errorPath), 0755); err != nil {
			return err
		}
		errorFile, err = os.Create(*errorPath + ".tmp")
		if err != nil {
			return err
		}
		defer func() { errorFile.Close(); os.Remove(*errorPath + ".tmp") }()
		options.Errors = errorFile
	}
	evaluation, err := nbamodel.Evaluate(rows, model, scoring, options)
	if err != nil {
		return err
	}
	if predictionFile != nil {
		if err := predictionFile.Close(); err != nil {
			return err
		}
		if err := os.Rename(temporary, *predictionPath); err != nil {
			return err
		}
	}
	if errorFile != nil {
		if err := errorFile.Close(); err != nil {
			return err
		}
		if err := os.Rename(*errorPath+".tmp", *errorPath); err != nil {
			return err
		}
	}
	if command == "train" {
		if err := writeJSON(*modelPath, model); err != nil {
			return err
		}
	}
	return emit(*reportPath, map[string]any{"dataset": *data, "dataset_sha256": digest, "availability_sha256": availabilityDigest, "availability_reports": len(availability), "coverage": cov, "model_training_examples": model.TrainingExamples, "model_trained_before": model.TrainedBefore, "selected_config": model.Config, "scoring": scoring, "validation_candidates": candidates, "evaluation": evaluation})
}

func splitIDs(value string) map[string]bool {
	ids := map[string]bool{}
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func emit(path string, value any) error {
	if path != "" {
		if err := writeJSON(path, value); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
