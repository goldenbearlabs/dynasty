package nhle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"crossover/internal/ingest"
)

func TestReportPagination(t *testing.T) {
	type row struct {
		PlayerID int `json:"playerId"`
	}
	var starts []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		starts = append(starts, start)
		if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("sort") != `[{"property":"playerId","direction":"ASC"}]` {
			t.Errorf("pagination parameters: %s", r.URL)
		}
		// Model the real feed's cap despite a larger total. McDavid is on a later page.
		data := []row{}
		for i := start; i < min(start+100, 241); i++ {
			data = append(data, row{PlayerID: i + 1})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data, "total": 241})
	}))
	defer server.Close()
	src := New(ingest.NewClient(0))
	src.StatsBase = server.URL
	rows, err := report[row](context.Background(), src, "skater/summary", 2027)
	if err != nil || len(rows) != 241 || rows[240].PlayerID != 241 || fmt.Sprint(starts) != "[0 100 200]" {
		t.Fatalf("rows=%d starts=%v err=%v", len(rows), starts, err)
	}
}

func TestReportRejectsIncompletePages(t *testing.T) {
	for _, scenario := range []string{"empty", "duplicate", "changed total", "missing total", "oversized", "http error"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("start") == "0" {
					fmt.Fprint(w, `{"total":2,"data":[{"playerId":1}]}`)
					return
				}
				switch scenario {
				case "empty":
					fmt.Fprint(w, `{"total":2,"data":[]}`)
				case "duplicate":
					fmt.Fprint(w, `{"total":2,"data":[{"playerId":1}]}`)
				case "changed total":
					fmt.Fprint(w, `{"total":3,"data":[{"playerId":2}]}`)
				case "missing total":
					fmt.Fprint(w, `{"data":[{"playerId":2}]}`)
				case "oversized":
					fmt.Fprint(w, `{"total":2,"data":[{"playerId":2},{"playerId":3}]}`)
				case "http error":
					http.Error(w, "unavailable", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			src := New(ingest.NewClient(0))
			src.StatsBase = server.URL
			rows, err := report[struct{ PlayerID int }](context.Background(), src, "skater/summary", 2027)
			if err == nil || rows != nil {
				t.Fatalf("partial report returned: rows=%v err=%v", rows, err)
			}
		})
	}
}
