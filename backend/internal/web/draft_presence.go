package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"sync"

	"crossover/internal/db"
	"crossover/internal/hub"
)

// Presence is transient, per draft, and separate from draft events. It never
// changes the database, clock, queues, or the existing draft websocket format.
type draftPresence struct {
	mu     sync.Mutex
	rooms  map[string]map[string]int // draft -> franchise -> open tabs
	events *hub.Hub
}

func newDraftPresence() *draftPresence {
	return &draftPresence{rooms: map[string]map[string]int{}, events: hub.New()}
}

// join counts connections rather than users so closing a second tab cannot
// mark a manager offline while their first tab is still connected.
func (p *draftPresence) join(room, franchise string) ([]byte, func()) {
	p.mu.Lock()
	if p.rooms[room] == nil {
		p.rooms[room] = map[string]int{}
	}
	p.rooms[room][franchise]++
	snapshot := p.snapshot(room)
	if p.rooms[room][franchise] == 1 {
		p.events.Publish(room, snapshot)
	}
	p.mu.Unlock()
	return snapshot, sync.OnceFunc(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.rooms[room][franchise]--
		if p.rooms[room][franchise] == 0 {
			delete(p.rooms[room], franchise)
			p.events.Publish(room, p.snapshot(room))
		}
		if len(p.rooms[room]) == 0 {
			delete(p.rooms, room)
		}
	})
}

// snapshot is called with the lock held, preserving event order across joins
// and leaves. Sorting makes each snapshot stable across reconnects.
func (p *draftPresence) snapshot(room string) []byte {
	ids := []string{}
	for id := range p.rooms[room] {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	body, _ := json.Marshal(struct {
		Online []string `json:"online_franchise_ids"`
	}{ids})
	return body
}

func (s *Server) watchDraftPresence(w http.ResponseWriter, r *http.Request, me db.Franchise, presence *draftPresence) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	draft, err := s.Queries.GetDraft(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if draft.DynastyID != me.DynastyID {
		writeError(w, http.StatusForbidden, "This draft belongs to another dynasty.")
		return
	}
	room := id.String()
	messages, unsubscribe := presence.events.Subscribe(room)
	defer unsubscribe()
	s.streamOnOpen(w, r, messages, func() ([]byte, func()) { return presence.join(room, me.ID.String()) })
}
