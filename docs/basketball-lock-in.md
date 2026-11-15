# Basketball lock-in

NBA, college basketball, and WNBA weekly scoring windows starting on or after
**November 16, 2026** use lock-in scoring. Earlier windows retain advance game
selection, including when their scores are recalculated. A doubled matchup
that began before the cutoff keeps its original rules for the whole matchup.

After a completed game with minutes played, a manager can lock in its fantasy
points until the player's next game starts or the scoring window ends,
whichever comes first. The player must have occupied a starting spot at tipoff.
Moving a player to the bench and back preserves this offer until its deadline;
promoting a player cannot make a game played on the bench eligible.

Players can move between compatible starting spots and the bench between
games. Both starters and bench players are held in place during their real
games. A manual lock-in holds the player in his spot for the rest of the scoring
window, including roster exits through drops, trades, and reserve moves.
The existing commissioner override remains available.

An ordinary week allows one locked game per player. A doubled matchup allows
two games over the whole matchup; the first manual lock already holds the
player's roster position. Remaining choices default to the latest eligible
completed games with positive minutes once the player has no unfinished games
left in the window, or the window ends. Bench games, zero-minute games, and
already locked games are skipped. Missing eligible games contribute zero;
negative fantasy scores and zero-point appearances with minutes still count.
A benched player has no automatic contribution. New scoring windows reset
manual locks while inheriting the starting lineup.

The lineup editor shows the game's points, a Lock in button, and its deadline.
Bench players with an active offer show a reminder to return to a starting spot.
Offers disappear at their deadline even when the score feed has not updated.

Implementation stores a baseline lineup and timestamped moves separately from
legacy date-based lineups. The editor and official scorer share the SQL
eligibility and fallback function in migration 0054. Lock-ins and moves invalidate
live responses and settled scoring caches in the same transaction. No migration
rewrites existing selections or scores.
