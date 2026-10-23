package web_test

import (
	"context"
	"net/http"
	"testing"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCollegeProspectsWaitForProfessionalRookieDraft(t *testing.T) {
	for _, sport := range []string{"nfl", "mlb", "wnba"} {
		t.Run(sport, func(t *testing.T) {
			pool := dbtest.Open(t)
			dbtest.Reset(t, pool)
			ctx := context.Background()
			marker := "professional_pool_" + sport
			t.Cleanup(func() {
				dbtest.Reset(t, pool)
				pool.Exec(ctx, `delete from players where note=$1`, marker)
			})
			server, registry := startServer(t, pool)
			ann := newBrowser(t, server.URL)
			c, _ := registry.Get(sport)
			ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
				Name: "Professional Pool", Leagues: []dynasty.LeagueSetup{{Competition: sport, Settings: c.Defaults}},
				Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
				Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
			}, nil)
			var view struct{ Leagues []struct{ ID string } }
			ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
			league := view.Leagues[0].ID
			ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
				{Competition: sport, FullName: "Zprofessional College Player", Note: marker},
				{Competition: sport, FullName: "Zprofessional Professional Player", Note: marker},
			}, nil)
			var collegeID, proID pgtype.UUID
			if err := pool.QueryRow(ctx, `update players set pool_eligible=false where note=$1 and full_name='Zprofessional College Player' returning id`, marker).Scan(&collegeID); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `select id from players where note=$1 and full_name='Zprofessional Professional Player'`, marker).Scan(&proID); err != nil {
				t.Fatal(err)
			}
			createDraft := func(kind string) string {
				t.Helper()
				var draft struct{ ID string }
				ann.want(http.StatusCreated, "POST", "/api/admin/drafts", map[string]any{
					"name": kind, "kind": kind, "year": 2027, "rounds": 1,
					"order": "linear", "league_ids": []string{league}, "pick_clock_seconds": 0,
				}, &draft)
				return draft.ID
			}
			startup := createDraft("startup")
			var page struct {
				Players []struct {
					FullName string `json:"full_name"`
				}
				Total int
			}
			ann.want(http.StatusOK, "GET", "/api/players?q=Zprofessional&draft_id="+startup, nil, &page)
			if len(page.Players) != 1 || page.Players[0].FullName != "Zprofessional Professional Player" || page.Total != 1 {
				t.Fatalf("startup pool includes college player or wrong count: %+v", page)
			}
			ann.want(http.StatusNoContent, "PUT", "/api/drafts/"+startup+"/queue", map[string]any{"player_ids": []pgtype.UUID{collegeID}}, nil)
			var queue []any
			ann.want(http.StatusOK, "GET", "/api/drafts/"+startup+"/queue", nil, &queue)
			if len(queue) != 0 {
				t.Fatal("college player entered draft queue")
			}
			ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+startup+"/start", nil, nil)
			ann.want(http.StatusUnprocessableEntity, "POST", "/api/drafts/"+startup+"/pick", map[string]any{"player_id": collegeID}, nil)
			ann.want(http.StatusNoContent, "POST", "/api/drafts/"+startup+"/pick", map[string]any{"player_id": proID}, nil)
			// Even a commissioner cannot directly acquire the college player.
			ann.want(http.StatusUnprocessableEntity, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": collegeID, "list": "reserve", "force": true}, nil)
			rookie := createDraft("seasonal")
			ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+rookie+"/start", nil, nil)
			ann.want(http.StatusUnprocessableEntity, "POST", "/api/drafts/"+rookie+"/pick", map[string]any{"player_id": collegeID}, nil)
			// The same query used by a qualifying professional prospect import
			// admits him without creating a new identity.
			if err := db.New(pool).UpdateProspect(ctx, db.UpdateProspectParams{ID: collegeID, FullName: "Zprofessional College Player", Positions: []string{}, Note: marker}); err != nil {
				t.Fatal(err)
			}
			ann.want(http.StatusNoContent, "POST", "/api/drafts/"+rookie+"/pick", map[string]any{"player_id": collegeID}, nil)
		})
	}
}
