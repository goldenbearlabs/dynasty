package web_test

import (
	"bytes"
	"context"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/lineup"
	"crossover/internal/players"
	"encoding/base64"
	"github.com/jackc/pgx/v5/pgtype"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

func TestTeamIdentityAndNicknames(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "identity_fixture"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note=$1`, marker) })
	server, registry := startServer(t, pool)
	admin, owner, other, guest := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	nhl, _ := registry.Get("nhl")
	admin.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{Name: "Identity Test", Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}, {Competition: "nhl", Settings: nhl.Defaults}}, Franchises: []dynasty.FranchiseSetup{{Name: "Admin", ManagerName: "Admin"}, {Name: "Owner", ManagerName: "Owner Manager"}, {Name: "Other", ManagerName: "Other Manager"}}, Account: dynasty.Account{Email: "identity-admin@example.com", Password: "correct horse"}}, nil)
	var invites []inviteRecord
	admin.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	var ownerID, otherID, ownerSlug, adminID string
	for _, i := range invites {
		if i.Name == "Admin" {
			adminID = i.ID
		}
		if i.Name == "Owner" {
			ownerID = i.ID
			ownerSlug = i.Slug
			owner.claim(i.InviteToken, "identity-owner@example.com")
		}
		if i.Name == "Other" {
			otherID = i.ID
			other.claim(i.InviteToken, "identity-other@example.com")
		}
	}
	q := db.New(pool)
	d, err := q.GetDynasty(ctx)
	if err != nil {
		t.Fatal(err)
	}
	leagues, err := q.ListLeagues(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	var nbaID, nhlID pgtype.UUID
	for _, l := range leagues {
		if l.Competition == "nba" {
			nbaID = l.ID
		} else {
			nhlID = l.ID
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, img)
	logo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	org := "/api/franchises/" + ownerID + "/identity"
	guest.want(http.StatusUnauthorized, "PUT", org, map[string]string{"name": "Unauthorized"}, nil)
	other.want(http.StatusForbidden, "PUT", org, map[string]string{"name": "Hijacked"}, nil)
	owner.want(http.StatusUnprocessableEntity, "PUT", org, map[string]string{"name": " "}, nil)
	owner.want(http.StatusUnprocessableEntity, "PUT", org, map[string]string{"name": "Owner", "image_url": "javascript:alert(1)"}, nil)
	owner.want(http.StatusUnprocessableEntity, "PUT", org, map[string]string{"name": "Owner", "image_url": "data:image/png;base64,bm90LWFuLWltYWdl"}, nil)
	owner.want(http.StatusBadRequest, "PUT", org, map[string]any{"name": "Owner", "is_commissioner": true}, nil)
	owner.want(http.StatusNoContent, "PUT", org, map[string]string{"name": "  North Star Sports  ", "image_url": logo}, nil)
	var me db.Franchise
	owner.want(http.StatusOK, "GET", "/api/session", nil, &me)
	if me.Name != "North Star Sports" || me.Slug != ownerSlug || me.ManagerName != "Owner Manager" || me.IsCommissioner || me.ImageUrl != logo {
		t.Fatalf("organization identity altered unrelated account fields: %+v", me)
	}
	teamURL := "/api/franchises/" + ownerID + "/leagues/" + nbaID.String() + "/identity"
	owner.want(http.StatusNoContent, "PUT", teamURL, map[string]string{"name": "North Star Hoops", "image_url": "https://example.com/hoops.png"}, nil)
	other.want(http.StatusForbidden, "PUT", teamURL, map[string]string{"name": "Hijacked"}, nil)
	other.want(http.StatusNoContent, "PUT", "/api/franchises/"+otherID+"/leagues/"+nbaID.String()+"/identity", map[string]string{"name": "Other Hoops", "image_url": logo}, nil)
	var franchise struct {
		Franchise db.Franchise `json:"franchise"`
		Rosters   []struct {
			LeagueID pgtype.UUID `json:"league_id"`
			TeamName string      `json:"team_name"`
			ImageURL string      `json:"image_url"`
			Players  []struct {
				FullName string `json:"full_name"`
				Nickname string `json:"nickname"`
			} `json:"players"`
		} `json:"rosters"`
	}
	guest.want(http.StatusOK, "GET", "/api/franchises/"+ownerSlug, nil, &franchise)
	for _, r := range franchise.Rosters {
		if r.LeagueID == nbaID && (r.TeamName != "North Star Hoops" || r.ImageURL != "https://example.com/hoops.png") {
			t.Fatal("per-sport identity missing")
		}
		if r.LeagueID == nhlID && (r.TeamName != me.Name || r.ImageURL != logo) {
			t.Fatal("organization fallback missing")
		}
	}
	l, _ := q.GetLeague(ctx, nbaID)
	if l.Name != "NBA" {
		t.Fatal("a manager renamed the shared league")
	}
	foreign := pgtype.UUID{}
	pool.QueryRow(ctx, `insert into dynasties(name,settings) values('Foreign','{}') returning id`).Scan(&foreign)
	var foreignLeague pgtype.UUID
	pool.QueryRow(ctx, `insert into leagues(dynasty_id,competition,name,settings) values($1,'nba','Foreign','{}') returning id`, foreign).Scan(&foreignLeague)
	owner.want(http.StatusForbidden, "PUT", "/api/franchises/"+ownerID+"/leagues/"+foreignLeague.String()+"/identity", map[string]string{"name": "Foreign"}, nil)
	var player pgtype.UUID
	if err := pool.QueryRow(ctx, `insert into players(competition,status,full_name,positions,note) values('nba','active','Zz Identity Player','{PG}',$1) returning id`, marker).Scan(&player); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into roster_entries(league_id,franchise_id,player_id,list,acquired_via) values($1,$2,$3,'main','manual')`, nbaID, me.ID, player); err != nil {
		t.Fatal(err)
	}
	nickURL := "/api/franchises/" + ownerID + "/players/" + player.String() + "/nickname"
	owner.want(http.StatusNoContent, "PUT", nickURL, map[string]string{"nickname": "  The North Star  "}, nil)
	other.want(http.StatusForbidden, "PUT", nickURL, map[string]string{"nickname": "Hijacked"}, nil)
	other.want(http.StatusNoContent, "PUT", "/api/franchises/"+otherID+"/players/"+player.String()+"/nickname", map[string]string{"nickname": "My Own Nickname"}, nil)
	owner.want(http.StatusUnprocessableEntity, "PUT", nickURL, map[string]string{"nickname": strings.Repeat("x", 41)}, nil)
	var nick map[string]string
	guest.want(http.StatusOK, "GET", nickURL, nil, &nick)
	if nick["nickname"] != "The North Star" {
		t.Fatal("nickname not stored")
	}
	var row db.Player
	row, err = q.GetPlayer(ctx, player)
	if err != nil || row.FullName != "Zz Identity Player" {
		t.Fatal("nickname replaced canonical name")
	}
	guest.want(http.StatusOK, "GET", "/api/franchises/"+ownerSlug, nil, &franchise)
	found := false
	for _, r := range franchise.Rosters {
		for _, p := range r.Players {
			if p.FullName == row.FullName && p.Nickname == "The North Star" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("public roster did not retain real name and nickname")
	}
	var starting lineup.View
	guest.want(http.StatusOK, "GET", "/api/leagues/"+nbaID.String()+"/lineup?franchise="+ownerID, nil, &starting)
	if len(starting.Players) != 1 || starting.Players[0].Nickname != "The North Star" {
		t.Fatal("lineup nickname missing")
	}
	// Duplicate record merges preserve each organization's nickname.
	var duplicate pgtype.UUID
	pool.QueryRow(ctx, `insert into players(competition,status,full_name,note) values('nba','active','Duplicate',$1) returning id`, marker).Scan(&duplicate)
	var otherUUID pgtype.UUID
	otherUUID.Scan(otherID)
	q.SetPlayerNickname(ctx, db.SetPlayerNicknameParams{FranchiseID: otherUUID, PlayerID: duplicate, Nickname: "Duplicate Name"})
	var adminUUID pgtype.UUID
	if err := adminUUID.Scan(adminID); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPlayerNickname(ctx, db.SetPlayerNicknameParams{FranchiseID: adminUUID, PlayerID: duplicate, Nickname: "Merged Nickname"}); err != nil {
		t.Fatal(err)
	}
	if err := players.NewService(pool, registry).Merge(ctx, player, duplicate); err != nil {
		t.Fatal(err)
	}
	moved, err := q.GetPlayerNickname(ctx, db.GetPlayerNicknameParams{FranchiseID: adminUUID, PlayerID: player})
	if err != nil || moved != "Merged Nickname" {
		t.Fatal("merge lost nickname from duplicate record")
	}
	kept, err := q.GetPlayerNickname(ctx, db.GetPlayerNicknameParams{FranchiseID: otherUUID, PlayerID: player})
	if err != nil || kept != "My Own Nickname" {
		t.Fatal("merge overwrote survivor nickname")
	}
	owner.want(http.StatusNoContent, "PUT", nickURL, map[string]string{"nickname": ""}, nil)
	guest.want(http.StatusOK, "GET", nickURL, nil, &nick)
	if nick["nickname"] != "" {
		t.Fatal("nickname could not be removed")
	}
	owner.want(http.StatusNoContent, "PUT", teamURL, map[string]string{"name": "", "image_url": ""}, nil)
	guest.want(http.StatusOK, "GET", "/api/franchises/"+ownerSlug, nil, &franchise)
	for _, r := range franchise.Rosters {
		if r.TeamName != me.Name || r.ImageURL != logo {
			t.Fatal("clearing team overrides did not restore organization defaults")
		}
	}
}

type inviteRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	InviteToken string `json:"invite_token"`
}
