package web

import (
	"net/http"
	"strconv"

	"crossover/internal/db"
	"crossover/internal/problem"
)

func (s *Server) notificationSummary(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	w.Header().Set("Cache-Control", "no-store")
	summary, err := s.Queries.NotificationSummary(r.Context(), me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	w.Header().Set("Cache-Control", "no-store")
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	kind := query.Get("kind")
	if kind != "" && kind != "roster" && kind != "trade" && kind != "target" {
		s.fail(w, r, problem.New("Unknown notification type."))
		return
	}
	unread := query.Get("unread") == "true"
	rows, err := s.Queries.ListNotifications(r.Context(), db.ListNotificationsParams{FranchiseID: me.ID, Kind: kind, UnreadOnly: unread, PageSize: 30, PageOffset: int32((page - 1) * 30)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	count, err := s.Queries.CountNotifications(r.Context(), db.CountNotificationsParams{FranchiseID: me.ID, Kind: kind, UnreadOnly: unread})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": rows, "total": count, "page": page, "per_page": 30})
}

func (s *Server) readNotification(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "Notification not found.")
		return
	}
	var body struct {
		Read bool `json:"read"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	n, err := s.Queries.ReadNotification(r.Context(), db.ReadNotificationParams{ID: id, FranchiseID: me.ID, IsRead: body.Read})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "Notification not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		ThroughID int64 `json:"through_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.ThroughID < 1 {
		s.fail(w, r, problem.New("Choose the latest notification you have seen."))
		return
	}
	s.done(w, r, s.Queries.ReadAllNotifications(r.Context(), db.ReadAllNotificationsParams{FranchiseID: me.ID, ThroughID: body.ThroughID}))
}
