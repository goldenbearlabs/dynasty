package web

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"crossover/internal/db"
	"crossover/internal/problem"
)

type identityInput struct {
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

func identityName(name string, required bool, limit int) (string, error) {
	name = strings.TrimSpace(name)
	if required && name == "" {
		return "", problem.New("Choose an organization name.")
	}
	if utf8.RuneCountInString(name) > limit {
		return "", problem.New("Names must be %d characters or fewer.", limit)
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", problem.New("Names must be on one line.")
	}
	return name, nil
}
func identityImage(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "data:") {
		if len(value) > 210000 {
			return "", problem.New("The image is too large. Upload a smaller image.")
		}
		header, data, ok := strings.Cut(value, ",")
		if !ok || header != "data:image/png;base64" && header != "data:image/jpeg;base64" {
			return "", problem.New("Upload a PNG or JPEG image.")
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(raw) > 150*1024 {
			return "", problem.New("The uploaded image is invalid or too large.")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		expected := "png"
		if header == "data:image/jpeg;base64" {
			expected = "jpeg"
		}
		if err != nil || format != expected || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1024 || cfg.Height > 1024 {
			return "", problem.New("Upload a valid image up to 1024 × 1024 pixels.")
		}
		if _, _, err := image.Decode(bytes.NewReader(raw)); err != nil {
			return "", problem.New("The uploaded image could not be read.")
		}
		return value, nil
	}
	u, err := url.Parse(value)
	if err != nil || len(value) > 2048 || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.ContainsFunc(value, unicode.IsSpace) {
		return "", problem.New("Use a complete http:// or https:// image URL, or upload an image.")
	}
	return value, nil
}
func (s *Server) editableIdentity(w http.ResponseWriter, r *http.Request, me db.Franchise) (db.Franchise, bool) {
	franchise, err := s.franchiseFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return db.Franchise{}, false
	}
	if franchise.DynastyID != me.DynastyID || franchise.ID != me.ID && !me.IsCommissioner {
		writeError(w, http.StatusForbidden, "You can only customize your own organization.")
		return db.Franchise{}, false
	}
	return franchise, true
}
func (s *Server) setOrganizationIdentity(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	f, ok := s.editableIdentity(w, r, me)
	if !ok {
		return
	}
	var body identityInput
	if !readJSON(w, r, &body) {
		return
	}
	name, err := identityName(body.Name, true, 80)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	img, err := identityImage(body.ImageURL)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Queries.UpdateOrganizationIdentity(r.Context(), db.UpdateOrganizationIdentityParams{ID: f.ID, Name: name, ImageUrl: img}))
}
func (s *Server) setTeamIdentity(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	f, ok := s.editableIdentity(w, r, me)
	if !ok {
		return
	}
	id, err := pathID(r, "league_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Queries.GetLeague(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l.DynastyID != f.DynastyID {
		writeError(w, http.StatusForbidden, "This league does not belong to your organization’s dynasty.")
		return
	}
	var body identityInput
	if !readJSON(w, r, &body) {
		return
	}
	name, err := identityName(body.Name, false, 80)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	img, err := identityImage(body.ImageURL)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Queries.SetTeamIdentity(r.Context(), db.SetTeamIdentityParams{FranchiseID: f.ID, LeagueID: l.ID, Name: name, ImageUrl: img}))
}
func (s *Server) getPlayerNickname(w http.ResponseWriter, r *http.Request) {
	f, err := s.franchiseFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := pathID(r, "player_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err = s.Queries.GetPlayer(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	nickname, err := s.Queries.GetPlayerNickname(r.Context(), db.GetPlayerNicknameParams{FranchiseID: f.ID, PlayerID: id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nickname": nickname})
}
func (s *Server) setPlayerNickname(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	f, ok := s.editableIdentity(w, r, me)
	if !ok {
		return
	}
	id, err := pathID(r, "player_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err = s.Queries.GetPlayer(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	var body struct {
		Nickname string `json:"nickname"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	nickname, err := identityName(body.Nickname, false, 40)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if nickname == "" {
		s.done(w, r, s.Queries.DeletePlayerNickname(r.Context(), db.DeletePlayerNicknameParams{FranchiseID: f.ID, PlayerID: id}))
		return
	}
	s.done(w, r, s.Queries.SetPlayerNickname(r.Context(), db.SetPlayerNicknameParams{FranchiseID: f.ID, PlayerID: id, Nickname: nickname}))
}
