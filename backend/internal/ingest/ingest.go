// Package ingest pulls real-world teams and players from public feeds into the database.
//
// Each feed implements Source and only translates its own JSON into the
// neutral Team and Player types. Syncer owns every database write.
package ingest

import (
	"context"
	"time"
)

// Source is one competition's feed.
type Source interface {
	Teams(ctx context.Context) ([]Team, error)
	Roster(ctx context.Context, team Team) ([]Player, error)
}

// PoolSource is a Source that deliberately covers only part of its league.
// Players already stored who fall outside it are removed, where an
// ordinary Source would only mark a player it no longer lists inactive.
type PoolSource interface {
	Pool() Pool
}

// Pool describes the part of a league a competition is limited to.
type Pool struct {
	Positions       []string // when set, only players at one of these positions belong
	ListedTeamsOnly bool     // when true, only players on the teams the Source lists belong
}

// ProspectSource is a feed of players who have not reached the competition
// yet: recruits, draft picks, players in a club's system.
type ProspectSource interface {
	Prospects(ctx context.Context) ([]Player, error)
}

// GameSource is a feed of games and what each player did in them.
type GameSource interface {
	// Games lists every game on one sports day.
	Games(ctx context.Context, day time.Time) ([]Game, error)
	// BoxScore returns a stat line for each player who appeared.
	BoxScore(ctx context.Context, game Game) ([]StatLine, error)
}

type Game struct {
	ProviderID string
	StartsAt   time.Time
	Status     string // GameScheduled | GameLive | GameFinal
	HomeTeam   string // the team's ProviderID
	AwayTeam   string
	HomeScore  int
	AwayScore  int
	Detail     string // the feed's short description of where the game stands: "7:32 - 3rd", "Final/OT"
}

const (
	GameScheduled = "scheduled"
	GameLive      = "live"
	GameFinal     = "final"
)

// StatLine is one player's game in the competition's canonical stat keys
// (the registry lists them), so one scoring rule works across feeds.
type StatLine struct {
	Provider   string
	ProviderID string // the player's id in the feed
	Stats      map[string]float64
}

// SeasonSource is a feed of season totals for a whole league at once,
// which is what makes it practical to keep every player's history.
type SeasonSource interface {
	// LatestSeason is the feed's number for the season being played, or the
	// last one played when none is.
	LatestSeason(now time.Time) int
	// SeasonStats returns every player's totals for one regular season.
	SeasonStats(ctx context.Context, year int) ([]SeasonLine, error)
}

// CareerSource is a feed of a player's seasons outside the league itself:
// the junior, college and international hockey behind an NHL prospect. It
// is asked about one player at a time.
type CareerSource interface {
	// IDSpace is the Provider whose player ids Career takes.
	IDSpace() string
	Career(ctx context.Context, providerID string) ([]Season, error)
}

// Season is one player's totals for one season with one team, in the
// competition's canonical stat keys.
type Season struct {
	Year   int    // the feed's number for the season, used for ordering
	Label  string // "2025-26", "2025"
	Team   string
	League string // empty for the competition's own league
	Games  int
	Stats  map[string]float64
}

// SeasonLine is a Season and whose it is.
type SeasonLine struct {
	Provider   string
	ProviderID string
	Season
}

type Team struct {
	ProviderID string
	Abbrev     string
	Name       string
	LogoURL    string
}

type Player struct {
	// Provider names the id space, e.g. "espn_basketball". A player keeps the
	// same Provider and ProviderID when he moves between competitions that
	// share an id space, which is how a CBB player becomes an NBA player.
	Provider    string
	ProviderID  string
	FullName    string
	Positions   []string
	BirthDate   time.Time // zero when the feed has none
	Class       string    // CBB only: FR, SO, JR, SR
	HeadshotURL string
	Note        string // prospects only: where he is now, e.g. his school or draft slot
	// Aliases are other ids this same person may already be stored under: a
	// feed that knows him by a different id, or by no id at all. If one
	// matches, he is the same row and gains this feed's id.
	Aliases []ExternalID
}

// ExternalID is a player's id in one feed's id space.
type ExternalID struct {
	Provider   string
	ProviderID string
}

// ParseDate reads the leading YYYY-MM-DD of a feed date, returning the zero
// time when it is missing or malformed.
func ParseDate(s string) time.Time {
	if len(s) < 10 {
		return time.Time{}
	}
	t, _ := time.Parse(time.DateOnly, s[:10])
	return t
}
