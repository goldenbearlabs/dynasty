package draft

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/db"
	"crossover/internal/hub"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/roster"
	"crossover/internal/settings"
)

type Service struct {
	// ClockInterval is how often RunClock looks for expired pick clocks.
	ClockInterval time.Duration

	pool *pgxpool.Pool
	hub  *hub.Hub
	log  *slog.Logger
}

func NewService(pool *pgxpool.Pool, hub *hub.Hub, log *slog.Logger) *Service {
	return &Service{ClockInterval: 2 * time.Second, pool: pool, hub: hub, log: log}
}

// State is everything the draft room shows.
type State struct {
	Draft     db.Draft               `json:"draft"`
	LeagueIDs []pgtype.UUID          `json:"league_ids"`
	Picks     []db.ListDraftPicksRow `json:"picks"`
	OnClock   pgtype.UUID            `json:"on_clock_pick_id"` // null when nobody is on the clock
}

func (s *Service) State(ctx context.Context, draftID pgtype.UUID) (State, error) {
	return loadState(ctx, db.New(s.pool), draftID)
}

func loadState(ctx context.Context, q *db.Queries, draftID pgtype.UUID) (State, error) {
	draft, err := q.GetDraft(ctx, draftID)
	if err != nil {
		return State{}, err
	}
	leagues, err := q.ListDraftLeagues(ctx, draftID)
	if err != nil {
		return State{}, err
	}
	picks, err := q.ListDraftPicks(ctx, draftID)
	if err != nil {
		return State{}, err
	}

	state := State{Draft: draft, Picks: picks, LeagueIDs: []pgtype.UUID{}}
	for _, l := range leagues {
		state.LeagueIDs = append(state.LeagueIDs, l.ID)
	}
	if draft.Status == "live" || draft.Status == "paused" {
		if pick := onClock(picks); pick != nil {
			state.OnClock = pick.ID
		}
	}
	return state, nil
}

// onClock is the next pick in order that is neither made nor skipped.
func onClock(picks []db.ListDraftPicksRow) *db.ListDraftPicksRow {
	for i := range picks {
		if !picks[i].PlayerID.Valid && !picks[i].SkippedAt.Valid {
			return &picks[i]
		}
	}
	return nil
}

// ---- running a draft ----

func (s *Service) Start(ctx context.Context, draftID pgtype.UUID) error {
	return s.setStatus(ctx, draftID, "scheduled", "live", "Only a draft that has not started can be started.")
}

func (s *Service) Pause(ctx context.Context, draftID pgtype.UUID) error {
	return s.setStatus(ctx, draftID, "live", "paused", "Only a live draft can be paused.")
}

// Resume restarts the clock in full for whoever is up.
func (s *Service) Resume(ctx context.Context, draftID pgtype.UUID) error {
	return s.setStatus(ctx, draftID, "paused", "live", "Only a paused draft can be resumed.")
}

func (s *Service) setStatus(ctx context.Context, draftID pgtype.UUID, from, to, refusal string) error {
	return s.change(ctx, draftID, true, func(q *db.Queries, draft db.Draft) error {
		if draft.Status != from {
			return problem.Error(refusal)
		}
		draft.Status = to
		return advance(ctx, q, draft, true)
	})
}

// Finish ends a draft early. Picks that were never made are forfeited.
func (s *Service) Finish(ctx context.Context, draftID pgtype.UUID) error {
	return s.change(ctx, draftID, true, func(q *db.Queries, draft db.Draft) error {
		if draft.Status != "live" && draft.Status != "paused" {
			return problem.New("Only a draft in progress can be finished.")
		}
		return complete(ctx, q, draft)
	})
}

// PickRequest is one manager choosing one player.
type PickRequest struct {
	DraftID  pgtype.UUID
	Actor    db.Franchise // who is clicking; a commissioner may pick for anyone
	PlayerID pgtype.UUID
	List     string      // where the player lands: main or reserve
	PickID   pgtype.UUID // optional: a specific pick, to make up one that was skipped
}

// Pick uses the franchise's pick on a player. Without a PickID it is the
// pick on the clock, or failing that the actor's earliest skipped pick.
func (s *Service) Pick(ctx context.Context, in PickRequest) error {
	return s.change(ctx, in.DraftID, false, func(q *db.Queries, draft db.Draft) error {
		if draft.Status != "live" {
			return problem.New("This draft is not live.")
		}
		picks, err := q.ListDraftPicks(ctx, in.DraftID)
		if err != nil {
			return err
		}
		current := onClock(picks)

		mine := func(p *db.ListDraftPicksRow) bool {
			return p.CurrentFranchiseID == in.Actor.ID || in.Actor.IsCommissioner
		}
		var pick *db.ListDraftPicksRow
		switch {
		case in.PickID.Valid:
			i := slices.IndexFunc(picks, func(p db.ListDraftPicksRow) bool { return p.ID == in.PickID })
			if i < 0 || picks[i].PlayerID.Valid || !mine(&picks[i]) {
				return problem.New("That pick is not yours to make.")
			}
			if current == nil || picks[i].ID != current.ID {
				if !picks[i].SkippedAt.Valid {
					return problem.New("That pick is not up yet.")
				}
			}
			pick = &picks[i]
		case current != nil && mine(current):
			pick = current
		default:
			i := slices.IndexFunc(picks, func(p db.ListDraftPicksRow) bool {
				return !p.PlayerID.Valid && p.SkippedAt.Valid && p.CurrentFranchiseID == in.Actor.ID
			})
			if i < 0 {
				return problem.New("It is not your pick.")
			}
			pick = &picks[i]
		}

		if err := makePick(ctx, q, draft, pick.ID, pick.CurrentFranchiseID, in.PlayerID, in.List, false); err != nil {
			return err
		}
		// Making up a skipped pick leaves the clock of whoever is up alone.
		return advance(ctx, q, draft, current != nil && pick.ID == current.ID)
	})
}

// Undo reverses the most recent pick: the player returns to the pool and
// the pick goes back on the clock.
func (s *Service) Undo(ctx context.Context, draftID pgtype.UUID) error {
	return s.change(ctx, draftID, true, func(q *db.Queries, draft db.Draft) error {
		if draft.Status != "live" && draft.Status != "paused" {
			return problem.New("Only a draft in progress can undo a pick.")
		}
		pick, err := q.LastMadeDraftPick(ctx, draftID)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("No pick has been made yet.")
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteRosterEntry(ctx, db.DeleteRosterEntryParams{
			LeagueID: pick.LeagueID, FranchiseID: pick.CurrentFranchiseID, PlayerID: pick.PlayerID,
		}); err != nil {
			return err
		}
		if err := lineup.Bench(ctx, q, pick.LeagueID, pick.CurrentFranchiseID, pick.PlayerID); err != nil {
			return err
		}
		if err := q.ClearDraftPick(ctx, pick.ID); err != nil {
			return err
		}
		if err := q.InsertTransaction(ctx, db.InsertTransactionParams{
			DynastyID: draft.DynastyID, LeagueID: pick.LeagueID, FranchiseID: pick.CurrentFranchiseID,
			Kind: "undo_pick", PlayerID: pick.PlayerID, DraftPickID: pick.ID, Detail: json.RawMessage(`{}`),
		}); err != nil {
			return err
		}
		return advance(ctx, q, draft, true)
	})
}

// makePick puts the player on the franchise's roster and records the pick,
// in the caller's transaction. The roster rules decide whether it is legal.
func makePick(ctx context.Context, q *db.Queries, draft db.Draft, pickID, franchiseID, playerID pgtype.UUID, list string, auto bool) error {
	player, err := q.GetPlayer(ctx, playerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.New("That player does not exist.")
	}
	if err != nil {
		return err
	}
	leagues, err := q.ListDraftLeagues(ctx, draft.ID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(leagues, func(l db.League) bool { return l.Competition == player.Competition })
	if i < 0 {
		return problem.New("%s is not in a sport this draft covers.", player.FullName)
	}
	franchise, err := q.GetFranchise(ctx, franchiseID)
	if err != nil {
		return err
	}

	if err := roster.AddIn(ctx, q, roster.Change{
		League: leagues[i], Franchise: franchise, PlayerID: playerID, List: list, Source: roster.Draft, PickID: pickID,
	}); err != nil {
		return err
	}
	if err := q.MakeDraftPick(ctx, db.MakeDraftPickParams{ID: pickID, PlayerID: playerID, LeagueID: leagues[i].ID, AutoPicked: auto}); err != nil {
		return err
	}
	return q.RemoveFromDraftQueues(ctx, db.RemoveFromDraftQueuesParams{DraftID: draft.ID, PlayerID: playerID})
}

// advance settles the draft after a change: it completes the draft when
// every pick is made, and otherwise sets the clock. restartClock gives
// whoever is up a full clock; without it the running clock is kept.
func advance(ctx context.Context, q *db.Queries, draft db.Draft, restartClock bool) error {
	picks, err := q.ListDraftPicks(ctx, draft.ID)
	if err != nil {
		return err
	}
	if draft.Status == "live" && !slices.ContainsFunc(picks, func(p db.ListDraftPicksRow) bool { return !p.PlayerID.Valid }) {
		return complete(ctx, q, draft)
	}

	clock := draft.ClockExpiresAt
	if restartClock {
		clock = pgtype.Timestamptz{}
		if draft.Status == "live" && draft.PickClockSeconds > 0 && onClock(picks) != nil {
			clock = pgtype.Timestamptz{Time: time.Now().Add(time.Duration(draft.PickClockSeconds) * time.Second), Valid: true}
		}
	}
	return q.SetDraftStatus(ctx, db.SetDraftStatusParams{ID: draft.ID, Status: draft.Status, ClockExpiresAt: clock})
}

// complete closes the draft and opens free agency for everyone it left
// undrafted.
func complete(ctx context.Context, q *db.Queries, draft db.Draft) error {
	if err := q.SetDraftStatus(ctx, db.SetDraftStatusParams{ID: draft.ID, Status: "complete"}); err != nil {
		return err
	}
	return q.MarkLeaguesDrafted(ctx, draft.ID)
}

// change runs fn with the draft locked, then tells the room what changed.
// structural says the change may touch many picks (an edit, an undo), so
// the room gets the whole state; otherwise it gets only what moved.
func (s *Service) change(ctx context.Context, draftID pgtype.UUID, structural bool, fn func(q *db.Queries, draft db.Draft) error) error {
	var before State
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		draft, err := q.LockDraft(ctx, draftID)
		if err != nil {
			return err
		}
		if before, err = loadState(ctx, q, draftID); err != nil {
			return err
		}
		return fn(q, draft)
	})
	if err != nil {
		return err
	}
	s.publish(ctx, draftID, before, structural)
	return nil
}

// ---- telling the room ----

// Message is what the draft room receives. A "state" message replaces
// everything; an "update" carries the draft header and only the picks that
// changed.
type Message struct {
	Type string `json:"type"` // state | update
	State
}

// Snapshot is the message a client gets when it joins.
func (s *Service) Snapshot(ctx context.Context, draftID pgtype.UUID) ([]byte, error) {
	state, err := s.State(ctx, draftID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Message{Type: "state", State: state})
}

// Watch subscribes to a draft's messages.
func (s *Service) Watch(draftID pgtype.UUID) (<-chan []byte, func()) {
	return s.hub.Subscribe(topic(draftID))
}

func (s *Service) publish(ctx context.Context, draftID pgtype.UUID, before State, structural bool) {
	after, err := s.State(context.WithoutCancel(ctx), draftID)
	if err != nil {
		s.log.Error("publish draft", "err", err)
		return
	}
	message := Message{Type: "state", State: after}
	if !structural && len(before.Picks) == len(after.Picks) {
		message.Type = "update"
		message.Picks = []db.ListDraftPicksRow{}
		for i, pick := range after.Picks {
			old := before.Picks[i]
			if pick.PlayerID != old.PlayerID || pick.SkippedAt != old.SkippedAt {
				message.Picks = append(message.Picks, pick)
			}
		}
	}
	raw, _ := json.Marshal(message)
	s.hub.Publish(topic(draftID), raw)
}

func topic(draftID pgtype.UUID) string {
	return "draft:" + draftID.String()
}

// ---- the clock ----

// RunClock watches for expired pick clocks until ctx ends. The deadline
// lives in the database, so nothing is lost across a restart.
func (s *Service) RunClock(ctx context.Context) {
	ticker := time.NewTicker(s.ClockInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expired, err := db.New(s.pool).ListExpiredDrafts(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.log.Error("list expired drafts", "err", err)
				}
				continue
			}
			for _, id := range expired {
				if err := s.expire(ctx, id); err != nil {
					s.log.Error("expire draft clock", "draft", id, "err", err)
				}
			}
		}
	}
}

// expire handles a clock that ran out: the franchise's queue makes the pick
// if it can, and otherwise the pick is skipped, to be made up later.
func (s *Service) expire(ctx context.Context, draftID pgtype.UUID) error {
	return s.change(ctx, draftID, false, func(q *db.Queries, draft db.Draft) error {
		// Someone may have picked between the check and the lock.
		if draft.Status != "live" || !draft.ClockExpiresAt.Valid || draft.ClockExpiresAt.Time.After(time.Now()) {
			return nil
		}
		picks, err := q.ListDraftPicks(ctx, draftID)
		if err != nil {
			return err
		}
		pick := onClock(picks)
		if pick == nil {
			return advance(ctx, q, draft, true)
		}

		picked, err := s.pickFromQueue(ctx, q, draft, pick)
		if err != nil {
			return err
		}
		if !picked {
			if err := q.SkipDraftPick(ctx, pick.ID); err != nil {
				return err
			}
		}
		return advance(ctx, q, draft, true)
	})
}

// pickFromQueue tries the franchise's queued players in order, main roster
// first and then reserve, and reports whether one of them was drafted.
func (s *Service) pickFromQueue(ctx context.Context, q *db.Queries, draft db.Draft, pick *db.ListDraftPicksRow) (bool, error) {
	queue, err := q.ListDraftQueue(ctx, db.ListDraftQueueParams{DraftID: draft.ID, FranchiseID: pick.CurrentFranchiseID})
	if err != nil {
		return false, err
	}
	for _, queued := range queue {
		for _, list := range []string{settings.ListMain, settings.ListReserve} {
			// Each attempt runs in a savepoint so a refused one leaves no trace.
			err := q.Savepoint(ctx, func(q *db.Queries) error {
				return makePick(ctx, q, draft, pick.ID, pick.CurrentFranchiseID, queued.PlayerID, list, true)
			})
			var refused problem.Error
			switch {
			case err == nil:
				return true, nil
			case !errors.As(err, &refused):
				return false, err
			}
		}
	}
	return false, nil
}

// ---- queues ----

func (s *Service) Queue(ctx context.Context, draftID, franchiseID pgtype.UUID) ([]db.ListDraftQueueRow, error) {
	return db.New(s.pool).ListDraftQueue(ctx, db.ListDraftQueueParams{DraftID: draftID, FranchiseID: franchiseID})
}

// SetQueue replaces a franchise's ranked wish list for a draft.
func (s *Service) SetQueue(ctx context.Context, draftID, franchiseID pgtype.UUID, playerIDs []pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if err := q.ClearDraftQueue(ctx, db.ClearDraftQueueParams{DraftID: draftID, FranchiseID: franchiseID}); err != nil {
			return err
		}
		for rank, playerID := range playerIDs {
			if err := q.InsertDraftQueue(ctx, db.InsertDraftQueueParams{
				DraftID: draftID, FranchiseID: franchiseID, PlayerID: playerID, Rank: int32(rank),
			}); err != nil {
				return problem.New("One of those players cannot be queued.")
			}
		}
		return nil
	})
}
