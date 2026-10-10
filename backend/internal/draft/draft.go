package draft

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
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

// autoPickDelay is how long a franchise with auto pick on is on the clock
// before its pick is made, so the room can see whose turn it was.
const autoPickDelay = 3 * time.Second

// autoPickPool is how many of the best players left in each sport auto pick
// chooses among when the franchise's queue has nobody it can take.
const autoPickPool = 30

type Service struct {
	// ClockInterval is how often RunClock looks for expired pick clocks.
	ClockInterval time.Duration

	pool *pgxpool.Pool
	hub  *hub.Hub
	log  *slog.Logger
}

func NewService(pool *pgxpool.Pool, hub *hub.Hub, log *slog.Logger) *Service {
	return &Service{ClockInterval: time.Second, pool: pool, hub: hub, log: log}
}

// State is everything the draft room shows.
type State struct {
	Draft     db.Draft               `json:"draft"`
	LeagueIDs []pgtype.UUID          `json:"league_ids"`
	Picks     []db.ListDraftPicksRow `json:"picks"`
	OnClock   pgtype.UUID            `json:"on_clock_pick_id"` // null when nobody is on the clock
	// AutoPick lists the franchises that have auto pick turned on.
	AutoPick []pgtype.UUID `json:"auto_pick_franchise_ids"`
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

	auto, err := q.ListDraftAutopick(ctx, draftID)
	if err != nil {
		return State{}, err
	}

	state := State{Draft: draft, Picks: picks, LeagueIDs: []pgtype.UUID{}, AutoPick: auto}
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
		if !picks[i].PlayerID.Valid && !picks[i].SkippedAt.Valid && !picks[i].PassedAt.Valid {
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
			if i < 0 || picks[i].PlayerID.Valid || picks[i].PassedAt.Valid || !mine(&picks[i]) {
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
				return !p.PlayerID.Valid && p.SkippedAt.Valid && !p.PassedAt.Valid && p.CurrentFranchiseID == in.Actor.ID
			})
			if i < 0 {
				return problem.New("It is not your pick.")
			}
			pick = &picks[i]
		}

		if err := makePick(ctx, q, draft, pick.ID, pick.CurrentFranchiseID, in.PlayerID, false); err != nil {
			return err
		}
		// Making up a skipped pick leaves the clock of whoever is up alone.
		return advance(ctx, q, draft, current != nil && pick.ID == current.ID)
	})
}

// Pass gives up the pick on the clock for good: the franchise takes nobody
// with it, and it cannot be made up later. Only a rookie draft allows it.
func (s *Service) Pass(ctx context.Context, draftID pgtype.UUID, actor db.Franchise) error {
	return s.change(ctx, draftID, false, func(q *db.Queries, draft db.Draft) error {
		switch {
		case draft.Status != "live":
			return problem.New("This draft is not live.")
		case draft.Kind == "startup":
			return problem.New("Picks cannot be passed in a startup draft.")
		}
		picks, err := q.ListDraftPicks(ctx, draftID)
		if err != nil {
			return err
		}
		current := onClock(picks)
		if current == nil || (current.CurrentFranchiseID != actor.ID && !actor.IsCommissioner) {
			return problem.New("It is not your pick.")
		}
		if err := q.PassDraftPick(ctx, current.ID); err != nil {
			return err
		}
		return advance(ctx, q, draft, true)
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
func makePick(ctx context.Context, q *db.Queries, draft db.Draft, pickID, franchiseID, playerID pgtype.UUID, auto bool) error {
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
	// A rookie draft is open to anyone unrostered, new to the pool or a free
	// agent, and holds its picks as rights until they are signed. A startup
	// draft fills both lists.
	startup := draft.Kind == "startup"
	list := settings.ListRights
	if startup {
		if list, err = startupList(ctx, q, leagues[i], franchiseID, player.Status == "prospect"); err != nil {
			return err
		}
	}

	if err := roster.AddIn(ctx, q, roster.Change{
		League: leagues[i], Franchise: franchise, PlayerID: playerID, List: list, Source: roster.Draft, PickID: pickID, Startup: startup,
	}); err != nil {
		return err
	}
	if err := q.MakeDraftPick(ctx, db.MakeDraftPickParams{ID: pickID, PlayerID: playerID, LeagueID: leagues[i].ID, AutoPicked: auto}); err != nil {
		return err
	}
	return q.RemoveFromDraftQueues(ctx, db.RemoveFromDraftQueuesParams{DraftID: draft.ID, PlayerID: playerID})
}

// startupList is where a startup pick lands: a prospect on the reserve list
// and anyone else on the main roster, or on the other list once his own is
// full. The franchise sets its reserve list as it likes afterwards, until
// the season starts.
func startupList(ctx context.Context, q *db.Queries, league db.League, franchiseID pgtype.UUID, prospect bool) (string, error) {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return "", err
	}
	rows, err := q.ListRosterEntries(ctx, db.ListRosterEntriesParams{LeagueID: league.ID, FranchiseID: franchiseID})
	if err != nil {
		return "", err
	}
	held := map[string]int{}
	for _, r := range rows {
		held[r.List]++
	}
	if prospect && held[settings.ListReserve] < rules.Roster.Reserve || held[settings.ListMain] >= rules.Roster.Main {
		return settings.ListReserve, nil
	}
	return settings.ListMain, nil
}

// advance settles the draft after a change: it completes the draft when
// every pick is made, and otherwise sets the clock. restartClock gives
// whoever is up a full clock; without it the running clock is kept.
func advance(ctx context.Context, q *db.Queries, draft db.Draft, restartClock bool) error {
	picks, err := q.ListDraftPicks(ctx, draft.ID)
	if err != nil {
		return err
	}
	if draft.Status == "live" && !slices.ContainsFunc(picks, func(p db.ListDraftPicksRow) bool { return !p.PlayerID.Valid && !p.PassedAt.Valid }) {
		return complete(ctx, q, draft)
	}

	clock := draft.ClockExpiresAt
	if restartClock {
		clock = pgtype.Timestamptz{}
		if pick := onClock(picks); draft.Status == "live" && pick != nil {
			// A franchise with auto pick on gets a few seconds, clock or no clock.
			auto, err := q.ListDraftAutopick(ctx, draft.ID)
			if err != nil {
				return err
			}
			wait := time.Duration(draft.PickClockSeconds) * time.Second
			if slices.Contains(auto, pick.CurrentFranchiseID) {
				wait = autoPickDelay
			}
			if wait > 0 {
				clock = pgtype.Timestamptz{Time: time.Now().Add(wait), Valid: true}
			}
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
	if err := q.MarkLeaguesDrafted(ctx, draft.ID); err != nil {
		return err
	}
	// The picks of a rookie draft now have their league's signing window.
	leagues, err := q.ListDraftLeagues(ctx, draft.ID)
	if err != nil {
		return err
	}
	days := 0
	for _, league := range leagues {
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return err
		}
		days = max(days, rules.Draft.SigningDays)
	}
	return q.StartSigningWindow(ctx, db.StartSigningWindowParams{
		DraftID: draft.ID, RightsUntil: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, days), Valid: true},
	})
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
	// A draft that has just finished leaves a year without one on the
	// books: the drafts further ahead are created so their picks can be traded.
	if after, err := db.New(s.pool).GetDraft(ctx, draftID); err == nil && after.Status == "complete" && before.Draft.Status != "complete" {
		if _, err := s.CreateFuture(ctx, after.DynastyID); err != nil {
			s.log.Warn("create future drafts", "err", err)
		}
	}
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
// if it can. Failing that a franchise with auto pick on takes one of the
// best players left, and anyone else's pick is skipped, to be made up later.
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
		if auto, err := q.ListDraftAutopick(ctx, draftID); err != nil {
			return err
		} else if !picked && slices.Contains(auto, pick.CurrentFranchiseID) {
			if picked, err = s.pickFromPool(ctx, q, draft, pick); err != nil {
				return err
			}
		}
		if !picked {
			if err := q.SkipDraftPick(ctx, pick.ID); err != nil {
				return err
			}
		}
		return advance(ctx, q, draft, true)
	})
}

// pickFromQueue tries the franchise's queued players in order and reports
// whether one of them was drafted.
func (s *Service) pickFromQueue(ctx context.Context, q *db.Queries, draft db.Draft, pick *db.ListDraftPicksRow) (bool, error) {
	queue, err := q.ListDraftQueue(ctx, db.ListDraftQueueParams{DraftID: draft.ID, FranchiseID: pick.CurrentFranchiseID})
	if err != nil {
		return false, err
	}
	players := make([]pgtype.UUID, len(queue))
	for i, queued := range queue {
		players[i] = queued.PlayerID
	}
	return pickFirst(ctx, q, draft, pick, players)
}

// pickFromPool drafts one of the best players left in the draft's sports,
// chosen at random: the roster rules refuse a sport the franchise has no
// room in, so the pick falls to one it does. It reports whether it found one.
func (s *Service) pickFromPool(ctx context.Context, q *db.Queries, draft db.Draft, pick *db.ListDraftPicksRow) (bool, error) {
	leagues, err := q.ListDraftLeagues(ctx, draft.ID)
	if err != nil {
		return false, err
	}
	var players []pgtype.UUID
	for _, league := range leagues {
		best, err := q.ListAutoPickCandidates(ctx, db.ListAutoPickCandidatesParams{Competition: league.Competition, PageSize: autoPickPool})
		if err != nil {
			return false, err
		}
		players = append(players, best...)
	}
	rand.Shuffle(len(players), func(i, j int) { players[i], players[j] = players[j], players[i] })
	return pickFirst(ctx, q, draft, pick, players)
}

// pickFirst drafts the first of the players the roster rules allow and
// reports whether there was one.
func pickFirst(ctx context.Context, q *db.Queries, draft db.Draft, pick *db.ListDraftPicksRow, players []pgtype.UUID) (bool, error) {
	for _, playerID := range players {
		// Each attempt runs in a savepoint so a refused one leaves no trace.
		err := q.Savepoint(ctx, func(q *db.Queries) error {
			return makePick(ctx, q, draft, pick.ID, pick.CurrentFranchiseID, playerID, true)
		})
		var refused problem.Error
		switch {
		case err == nil:
			return true, nil
		case !errors.As(err, &refused):
			return false, err
		}
	}
	return false, nil
}

// SetAutoPick turns a franchise's auto pick on or off for a draft. If the
// franchise is on the clock, its clock starts again to match.
func (s *Service) SetAutoPick(ctx context.Context, draftID, franchiseID pgtype.UUID, on bool) error {
	return s.change(ctx, draftID, false, func(q *db.Queries, draft db.Draft) error {
		if draft.Status == "complete" {
			return problem.New("This draft is over.")
		}
		var err error
		if on {
			err = q.SetDraftAutopick(ctx, db.SetDraftAutopickParams{DraftID: draftID, FranchiseID: franchiseID})
		} else {
			err = q.ClearDraftAutopick(ctx, db.ClearDraftAutopickParams{DraftID: draftID, FranchiseID: franchiseID})
		}
		if err != nil {
			return err
		}
		picks, err := q.ListDraftPicks(ctx, draftID)
		if err != nil {
			return err
		}
		pick := onClock(picks)
		return advance(ctx, q, draft, pick != nil && pick.CurrentFranchiseID == franchiseID)
	})
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
