// Package dynasty creates and maintains the league structure: the dynasty,
// its leagues and their settings, and its franchises.
package dynasty

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/auth"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
)

type Service struct {
	pool     *pgxpool.Pool
	registry competition.Registry
}

func NewService(pool *pgxpool.Pool, registry competition.Registry) *Service {
	return &Service{pool: pool, registry: registry}
}

// Setup is everything the first-run wizard collects.
type Setup struct {
	Name       string           `json:"name"`
	Settings   settings.Dynasty `json:"settings"`
	Leagues    []LeagueSetup    `json:"leagues"`
	Franchises []FranchiseSetup `json:"franchises"` // the first is the commissioner
	Account    Account          `json:"account"`    // the commissioner's sign-in
}

type Account struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LeagueSetup struct {
	Competition string          `json:"competition"`
	Settings    settings.League `json:"settings"`
}

type FranchiseSetup struct {
	Name        string `json:"name"`
	ManagerName string `json:"manager_name"`
}

// Create builds the dynasty in one transaction and returns the
// commissioner's new account. Every other franchise gets an invite link.
func (s *Service) Create(ctx context.Context, in Setup) (db.User, error) {
	if strings.TrimSpace(in.Name) == "" {
		return db.User{}, problem.New("The dynasty needs a name.")
	}
	if len(in.Leagues) == 0 {
		return db.User{}, problem.New("Pick at least one sport.")
	}
	if len(in.Franchises) == 0 {
		return db.User{}, problem.New("Add at least one franchise.")
	}
	if err := in.Settings.Validate(); err != nil {
		return db.User{}, problem.Error(err.Error())
	}
	var keys []string
	for _, l := range in.Leagues {
		if slices.Contains(keys, l.Competition) {
			return db.User{}, problem.New("%s is listed twice.", l.Competition)
		}
		keys = append(keys, l.Competition)
	}
	for _, l := range in.Leagues {
		if err := s.validate(l.Competition, l.Settings, keys); err != nil {
			return db.User{}, err
		}
	}

	var commissioner db.User
	err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if _, err := q.GetDynasty(ctx); !errors.Is(err, pgx.ErrNoRows) {
			if err != nil {
				return err
			}
			return problem.New("This server already has a dynasty.")
		}

		dynastySettings, _ := json.Marshal(in.Settings)
		dynasty, err := q.CreateDynasty(ctx, db.CreateDynastyParams{Name: strings.TrimSpace(in.Name), Settings: dynastySettings})
		if err != nil {
			return err
		}
		for _, l := range in.Leagues {
			c, _ := s.registry.Get(l.Competition)
			leagueSettings, _ := json.Marshal(l.Settings)
			if _, err := q.CreateLeague(ctx, db.CreateLeagueParams{
				DynastyID: dynasty.ID, Competition: c.Key, Name: c.Name, Settings: leagueSettings,
			}); err != nil {
				return err
			}
		}
		for i, f := range in.Franchises {
			franchise, err := addFranchise(ctx, q, dynasty.ID, f, i == 0)
			if err != nil {
				return err
			}
			if i == 0 {
				if commissioner, err = auth.CreateUser(ctx, q, in.Account.Email, in.Account.Password); err != nil {
					return err
				}
				if err := q.ClaimFranchise(ctx, db.ClaimFranchiseParams{ID: franchise.ID, UserID: commissioner.ID}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return commissioner, err
}

// UpdateLeagueSettings validates and stores a league's rules.
func (s *Service) UpdateLeagueSettings(ctx context.Context, league db.League, rules settings.League) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		leagues, err := q.ListLeagues(ctx, league.DynastyID)
		if err != nil {
			return err
		}
		var keys []string
		for _, l := range leagues {
			keys = append(keys, l.Competition)
		}
		if err := s.validate(league.Competition, rules, keys); err != nil {
			return err
		}

		raw, _ := json.Marshal(rules)
		if err := q.UpdateLeagueSettings(ctx, db.UpdateLeagueSettingsParams{ID: league.ID, Settings: raw}); err != nil {
			return err
		}
		return q.InsertTransaction(ctx, db.InsertTransactionParams{
			DynastyID: league.DynastyID, LeagueID: league.ID, Kind: "settings", Detail: json.RawMessage(`{}`),
		})
	})
}

// validate checks one league's rules against its sport and its sibling leagues.
func (s *Service) validate(key string, rules settings.League, dynastyKeys []string) error {
	c, ok := s.registry.Get(key)
	if !ok {
		return problem.New("Unknown sport %q.", key)
	}
	others := slices.DeleteFunc(slices.Clone(dynastyKeys), func(k string) bool { return k == key })
	if err := rules.Validate(c.Catalog(others)); err != nil {
		return problem.New("%s: %s", c.Name, err)
	}
	return nil
}

// AddFranchise adds a manager to an existing dynasty.
func (s *Service) AddFranchise(ctx context.Context, dynastyID pgtype.UUID, f FranchiseSetup) (db.Franchise, error) {
	var franchise db.Franchise
	err := db.InTx(ctx, s.pool, func(q *db.Queries) (err error) {
		franchise, err = addFranchise(ctx, q, dynastyID, f, false)
		return err
	})
	return franchise, err
}

// UpdateFranchise renames a franchise or changes its commissioner flag,
// never leaving the dynasty without a commissioner.
func (s *Service) UpdateFranchise(ctx context.Context, franchise db.Franchise, f FranchiseSetup, isCommissioner bool) error {
	if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.ManagerName) == "" {
		return problem.New("A franchise needs a name and a manager.")
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if franchise.IsCommissioner && !isCommissioner {
			all, err := q.ListFranchises(ctx, franchise.DynastyID)
			if err != nil {
				return err
			}
			others := slices.ContainsFunc(all, func(o db.Franchise) bool { return o.IsCommissioner && o.ID != franchise.ID })
			if !others {
				return problem.New("A dynasty needs at least one commissioner.")
			}
		}
		return q.UpdateFranchise(ctx, db.UpdateFranchiseParams{
			ID: franchise.ID, Name: strings.TrimSpace(f.Name), ManagerName: strings.TrimSpace(f.ManagerName), IsCommissioner: isCommissioner,
		})
	})
}

// ReissueInvite gives a franchise a fresh invite link, replacing any old
// one. Opening it sets (or resets) the account that runs the franchise.
func (s *Service) ReissueInvite(ctx context.Context, franchiseID pgtype.UUID) (string, error) {
	token := newToken()
	return token, db.New(s.pool).SetFranchiseToken(ctx, db.SetFranchiseTokenParams{
		ID: franchiseID, InviteToken: pgtype.Text{String: token, Valid: true},
	})
}

func addFranchise(ctx context.Context, q *db.Queries, dynastyID pgtype.UUID, f FranchiseSetup, isCommissioner bool) (db.Franchise, error) {
	name, manager := strings.TrimSpace(f.Name), strings.TrimSpace(f.ManagerName)
	if name == "" || manager == "" {
		return db.Franchise{}, problem.New("A franchise needs a name and a manager.")
	}

	// Slugs appear in URLs and must be unique within the dynasty.
	existing, err := q.ListFranchises(ctx, dynastyID)
	if err != nil {
		return db.Franchise{}, err
	}
	base := slugify(name)
	slug := base
	for n := 2; slices.ContainsFunc(existing, func(o db.Franchise) bool { return o.Slug == slug }); n++ {
		slug = base + "-" + strconv.Itoa(n)
	}

	return q.CreateFranchise(ctx, db.CreateFranchiseParams{
		DynastyID:      dynastyID,
		Name:           name,
		ManagerName:    manager,
		Slug:           slug,
		InviteToken:    pgtype.Text{String: newToken(), Valid: true},
		IsCommissioner: isCommissioner,
	})
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	slug := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		return "franchise"
	}
	return slug
}

// newToken returns an unguessable invite token.
func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
