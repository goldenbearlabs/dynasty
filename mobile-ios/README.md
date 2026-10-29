<!-- Explains how to run the iPhone SwiftUI app and where its first feature slice lives. -->
# Mosaic Fantasy for iPhone

This is the new native iPhone client. It sits beside `mobile/`, which remains the Flutter reference while native screens are built. After selecting a league, the app has Teams, Standings, Matchups, Players, and More tabs. Standings includes an always-visible Overall pill, scrollable sport pills with glowing dots, sorting, odds, season context, and matchup previews. Teams opens a franchise overview and sport team pages with manager roster and lineup controls. Matchups and Players still have standalone placeholders; team pages can open their own matchup and player details. This work does not change the Go API or the live database.

## Run it

1. Start the local Go API from the repository root with `make api`. Debug expects `http://localhost:8090` unless you set `API_BASE_URL` in the Xcode scheme's Run environment variables. A simulator shares your Mac's localhost; a physical iPhone does not.
2. Open `mobile-ios/MosaicFantasy.xcodeproj` in Xcode, choose the **MosaicFantasy** scheme and one iPhone simulator, and press **Run** (⌘R). Leave Flutter's simulator closed to save memory. Xcode can rebuild and relaunch without rebooting the simulator.
3. To inspect a screen without starting the API, open its Swift file and select a `#Preview` canvas. Leagues and League each have loaded, empty, loading, and error examples. Keep either one canvas or one simulator active on a low-memory Mac.

The Debug app uses bundle ID `com.mosaicfantasy.mosaicFantasy.swiftdev`, so it can coexist with the existing app. Release uses `com.mosaicfantasy.mosaicFantasy` and `https://mosaicfantasy.com`. Only Debug allows local HTTP networking. This is an iPhone-only iOS 17+ target. The new account endpoints need to be deployed before testing those actions against production.

## What the files do

| File | Purpose |
| --- | --- |
| `MosaicFantasyApp.swift` | Starts the app, restores the session, and chooses account entry or Leagues. |
| `Models.swift` | Decodes account, league, roster, lineup, player, and matchup JSON from Go. |
| `APIClient.swift` | Sends shared `URLSession` requests and checks dynasty IDs before team reads and writes. |
| `SessionStore.swift` | Refreshes mobile sessions and stores the bearer token in Keychain. |
| `Theme.swift` | Holds website colors and reusable sport dots and state panels. |
| `AuthView.swift` | Handles sign-in, account creation, invite claim, and reset requests. |
| `LeaguesView.swift` | Lists playable memberships, offers invite guidance, and opens verified leagues. |
| `LeagueTabs.swift` | Holds the five native tabs after a playable league is selected. |
| `LeagueView.swift` | Loads the verified Standings dashboard, filters activity, and refreshes its sections. |
| `StandingsView.swift` | Shows Overall and sport standings, sorting, odds, champions, cutoff, explanations, and franchise summaries. |
| `TeamsView.swift` | Shows franchise identities, sport team summaries, season tasks, and rookie draft picks; switches to other franchises read-only. |
| `SportTeamView.swift` | Shows one sport's roster, standings, matchup and schedule context, and the manager's roster actions. |
| `LineupEditor.swift` | Edits daily or weekly starter slots, counted games, auto-set, pitcher starts, and performance views. |
| `RosterEditors.swift` | Handles whole-list changes, injury swaps, and player nicknames in focused sheets. |
| `TeamPlayerSearch.swift` | Searches sport players, opens profiles, and adds free agents or submits waiver claims. |
| `TeamIdentityEditor.swift` | Edits franchise or sport team names and images, including a resized Photos upload. |
| `SeasonRosterEditor.swift` | Moves signed players between main and reserve and confirms the season's one-time save. |
| `MatchupsSection.swift` | Shows head-to-head periods, scores, and starter contributions under sport standings. |
| `ScoreFeed.swift` | Opens authenticated score WebSockets for the future Scores screen and can refresh it after score changes or reconnects. |
| `MoreView.swift`, `AccountView.swift` | Provide additional league links, account status, and sign out. Leagues opens Account directly from its avatar. |
| `PreviewFixtures.swift` | Supplies sample UI states without a server. |
| `MosaicFantasyTests.swift` | Checks JSON, token lifecycle, invite endpoints, and league safety. |
| `Info-Debug.plist`, `Info-Release.plist` | Set app names and limit the HTTP allowance to Debug. |
| `MosaicFantasy.xcodeproj/` | Defines the iPhone app and test targets, bundle IDs, and shared Run scheme. |
| `PRODUCT.md` | Records the agreed audience, visual direction, and first-slice constraints for UI work. |
| `.gitignore` | Excludes generated Xcode state and build files. |

For example, Teams checks `GET /api/dynasty` against the selected membership ID, then asks `GET /api/franchises/{slug}` for the chosen franchise. Go loads its sport rosters and draft picks from Postgres; `Models.swift` decodes the response, and `TeamsView.swift` shows the sport cards. Opening a sport requests `GET /api/leagues/{id}/lineup?franchise=...`. A manager's lineup save sends `PUT /api/leagues/{id}/lineup` with the day and starter entries; Go checks the signed-in franchise, slot eligibility, and locks before storing it. The Swift client never sends commissioner override fields. A rival's team can be viewed but has no roster or lineup editor. Standings uses pull-to-refresh; it does not open live score sockets. The future Scores screen can use `ScoreFeed` with the current mobile token and restart the feed when that token changes.

## Focused tests

From `mobile-ios/`, run:

```sh
xcodebuild -project MosaicFantasy.xcodeproj -scheme MosaicFantasy \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
  -parallel-testing-enabled NO -jobs 1 test
```

Xcode's **Test** action (⌘U) runs the same suite. Use one simulator at a time. Debug-only `MOSAIC_SHOWCASE=leagues`, `MOSAIC_SHOWCASE=league`, `MOSAIC_SHOWCASE=teams`, or `MOSAIC_SHOWCASE=sport-team` in the Run environment displays sample screens for visual checks; remove it before testing the real API.
