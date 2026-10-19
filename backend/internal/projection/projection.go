// Package projection estimates a player's fantasy points in a game still
// to be played. There is no model of its own: a player's recent seasons
// give what he does in a game he plays, and the betting market's line on
// the game moves that up or down. The market already weighs the opponent,
// home ice, rest and injuries.
package projection

// Season is one season of a player's totals in his competition.
type Season struct {
	Year  int
	Games float64 // his appearances
	Stats map[string]float64
}

// Game is a game still to be played, as the player's side sees it. The
// odds are zero when no line is posted.
type Game struct {
	TeamScore, OpponentScore float64 // each side's implied score
	Win                      float64 // his side's chance of winning
	Average                  float64 // the average side's implied score across games with a line
	Starter                  string  // confirmed, expected, other (a teammate is), or empty
	Injured                  bool
}

// lastSeason is how much a game from the season before counts against a
// game from the latest one.
const lastSeason = 0.25

// Baseline is what a player does in a game he plays, stat by stat, and the
// share of his side's games he plays. lengths is how many games each of the
// competition's latest seasons has run. Two seasons are blended, the
// earlier counting a quarter as much, so a hot or slow start is tempered by
// a full season.
func Baseline(seasons []Season, lengths map[int]float64) (perGame map[string]float64, plays float64) {
	latest := 0
	for year := range lengths {
		latest = max(latest, year)
	}
	weight := func(year int) float64 {
		if year < latest {
			return lastSeason
		}
		return 1
	}
	var games, length float64
	for year, l := range lengths {
		length += weight(year) * l
	}
	perGame = map[string]float64{}
	for _, s := range seasons {
		if _, ok := lengths[s.Year]; !ok {
			continue
		}
		games += weight(s.Year) * s.Games
		for k, v := range s.Stats {
			perGame[k] += weight(s.Year) * v
		}
	}
	if games == 0 || length == 0 {
		return map[string]float64{}, 0
	}
	for k := range perGame {
		perGame[k] /= games
	}
	return perGame, min(1, games/length)
}

// Points is a player's projected fantasy points in one game under a
// league's scoring. follows says which stats move with the opponent's
// implied score or the chance of winning; the rest move with his own side's.
// A named starter plays; an injured player, or one whose teammate is named
// to start in his place, does not.
func Points(perGame map[string]float64, plays float64, g Game, scoring map[string]float64, follows map[string]string) float64 {
	switch {
	case g.Injured || g.Starter == "other":
		return 0
	case g.Starter == "confirmed" || g.Starter == "expected":
		plays = 1
	}
	total := 0.0
	for stat, points := range scoring {
		total += points * perGame[stat] * factor(follows[stat], g)
	}
	return total * plays
}

// factor is how much the line moves one stat, held between half and half
// again so one odd line cannot swamp a projection. Without a line it is 1.
func factor(follows string, g Game) float64 {
	ratio := func(x, of float64) float64 {
		if x <= 0 || of <= 0 {
			return 1
		}
		return min(1.5, max(0.5, x/of))
	}
	switch follows {
	case "none":
		return 1
	case "opponent":
		return ratio(g.OpponentScore, g.Average)
	case "win":
		return ratio(g.Win, 0.5)
	case "loss":
		if g.Win <= 0 {
			return 1
		}
		return ratio(1-g.Win, 0.5)
	default:
		return ratio(g.TeamScore, g.Average)
	}
}
