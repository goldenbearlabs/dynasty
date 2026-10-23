// Saves a private league setup from the original rules without touching live gameplay.

package dynasty

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/problem"
)

type PresetLeagueInput struct {
	Name          string `json:"name"`
	FranchiseName string `json:"franchise_name"`
	ManagerName   string `json:"manager_name"`
}

type Membership struct {
	ID             pgtype.UUID `json:"id"`
	Name           string      `json:"name"`
	FranchiseID    pgtype.UUID `json:"franchise_id"`
	FranchiseName  string      `json:"franchise_name"`
	IsCommissioner bool        `json:"is_commissioner"`
	Sports         []string    `json:"sports"`
	Ready          bool        `json:"ready"`
}

func (s *Service) CreatePresetLeague(ctx context.Context, userID pgtype.UUID, in PresetLeagueInput) (pgtype.UUID, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.FranchiseName = strings.TrimSpace(in.FranchiseName)
	in.ManagerName = strings.TrimSpace(in.ManagerName)
	if in.Name == "" || in.FranchiseName == "" || in.ManagerName == "" {
		return pgtype.UUID{}, problem.New("Enter a league name, franchise name, and manager name.")
	}
	if len(in.Name) > 100 || len(in.FranchiseName) > 100 || len(in.ManagerName) > 100 {
		return pgtype.UUID{}, problem.New("Names must be 100 characters or fewer.")
	}

	var id pgtype.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var templateID pgtype.UUID
		var settings []byte
		if err := tx.QueryRow(ctx, `select id, settings from dynasties order by created_at, id limit 1`).Scan(&templateID, &settings); err != nil {
			if err == pgx.ErrNoRows {
				return problem.New("The league preset is not available yet.")
			}
			return err
		}
		var snapshot []byte
		if err := tx.QueryRow(ctx, `select coalesce(jsonb_agg(jsonb_build_object('competition', competition, 'name', name, 'settings', settings) order by competition), '[]'::jsonb)
			from leagues where dynasty_id = $1`, templateID).Scan(&snapshot); err != nil {
			return err
		}
		if string(snapshot) == "[]" {
			return problem.New("The league preset has no sports.")
		}
		return tx.QueryRow(ctx, `insert into league_workspaces
			(owner_user_id, name, franchise_name, manager_name, dynasty_settings, leagues_snapshot)
			values ($1, $2, $3, $4, $5, $6) returning id`,
			userID, in.Name, in.FranchiseName, in.ManagerName, settings, snapshot).Scan(&id)
	})
	return id, err
}

func (s *Service) ListMemberships(ctx context.Context, userID pgtype.UUID) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `select d.id, d.name, f.id, f.name, f.is_commissioner,
		array(select l.competition from leagues l where l.dynasty_id = d.id order by l.competition)
		from franchises f join dynasties d on d.id = f.dynasty_id
		where f.user_id = $1 order by d.created_at, d.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memberships := []Membership{}
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.ID, &m.Name, &m.FranchiseID, &m.FranchiseName, &m.IsCommissioner, &m.Sports); err != nil {
			return nil, err
		}
		m.Ready = true
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	workspaces, err := s.pool.Query(ctx, `select id, name, franchise_id, franchise_name, leagues_snapshot
		from league_workspaces where owner_user_id = $1 order by created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer workspaces.Close()
	for workspaces.Next() {
		var m Membership
		var snapshot []byte
		if err := workspaces.Scan(&m.ID, &m.Name, &m.FranchiseID, &m.FranchiseName, &snapshot); err != nil {
			return nil, err
		}
		m.IsCommissioner = true
		if err := snapshotSports(snapshot, &m.Sports); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	return memberships, workspaces.Err()
}

func (s *Service) GetMembership(ctx context.Context, userID, leagueID pgtype.UUID) (Membership, error) {
	var m Membership
	err := s.pool.QueryRow(ctx, `select d.id, d.name, f.id, f.name, f.is_commissioner,
		array(select l.competition from leagues l where l.dynasty_id = d.id order by l.competition)
		from franchises f join dynasties d on d.id = f.dynasty_id
		where f.user_id = $1 and d.id = $2`, userID, leagueID).Scan(
		&m.ID, &m.Name, &m.FranchiseID, &m.FranchiseName, &m.IsCommissioner, &m.Sports,
	)
	if err == nil {
		m.Ready = true
		return m, nil
	}
	if err != pgx.ErrNoRows {
		return Membership{}, err
	}
	var snapshot []byte
	err = s.pool.QueryRow(ctx, `select id, name, franchise_id, franchise_name, leagues_snapshot
		from league_workspaces where owner_user_id = $1 and id = $2`, userID, leagueID).Scan(
		&m.ID, &m.Name, &m.FranchiseID, &m.FranchiseName, &snapshot,
	)
	if err != nil {
		return Membership{}, err
	}
	m.IsCommissioner = true
	return m, snapshotSports(snapshot, &m.Sports)
}

func snapshotSports(raw []byte, sports *[]string) error {
	var leagues []struct {
		Competition string `json:"competition"`
	}
	if err := json.Unmarshal(raw, &leagues); err != nil {
		return err
	}
	*sports = make([]string, 0, len(leagues))
	for _, league := range leagues {
		*sports = append(*sports, league.Competition)
	}
	return nil
}
