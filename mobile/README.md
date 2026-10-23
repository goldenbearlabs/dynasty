<!-- Explains how to run the iOS app and trace its league and account requests. -->

# Mosaic Fantasy mobile

This Flutter project is the iPhone/iPad client for the existing Go API. Signed-out people see sign-in and account creation. A new account without a league sees a small account lobby; an existing manager sees the dark “My Leagues” page with their sport franchises and rules. Team rosters, players, and scores remain read-only; roster-changing and commissioner tools are later chunks.

## Run it

1. In the repository root, start the database and API using the existing README instructions (`make db`, then `make api`).
2. Start an iOS simulator in Xcode.
3. In this directory, run:

```sh
flutter run --dart-define=API_BASE_URL=http://localhost:8090
```

The iOS simulator can use the Mac's `localhost`. A physical iPhone needs your Mac's LAN address, a reachable HTTPS development server, and any required iOS local-network permissions. If you omit `API_BASE_URL`, the app uses `https://mosaicfantasy.com`; release builds therefore use production by default. The new mobile account endpoints must be deployed to that server before production sign-in or registration will work.

Account creation needs an email sender. For local development only, run `make web` in one terminal and start the API with `PUBLIC_URL=http://localhost:5173 DEV_LOG_EMAILS=1 make api` in another; the verification link will appear in the API terminal and open the web frontend. You can stop the simulator while verifying to save memory. Production requires `PUBLIC_URL`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, and `MAIL_FROM` instead. Do not enable `DEV_LOG_EMAILS` in production. The current chunk lets a verified account claim a commissioner-prepared franchise invite; multi-league creation and shared join codes are still being built.

Run `flutter test` for the API and layout checks, `flutter analyze` for static checks, and `flutter doctor -v` for toolchain status. Android setup is intentionally postponed until after the iOS release.

## How this chunk works

`lib/main.dart` owns the dark theme and chooses bottom navigation on iPhone or a side rail on iPad. `lib/config/app_config.dart` picks the API address at build time. `lib/data/api_client.dart` sends HTTP requests; `lib/data/models.dart` parses the JSON returned by Go. `lib/data/session_controller.dart` keeps the manager's revocable token in iOS Keychain storage and restores or refreshes it. `lib/screens/home_screen.dart` puts the league and team buttons above the organized rule book; the other screen files render their own area.

For example, signing in sends your email and password to `POST /api/mobile/auth/login`. Go checks the account in PostgreSQL and returns a revocable mobile token. Flutter stores only the token in Keychain. If that account manages a franchise, Home then requests `GET /api/dynasty` and `GET /api/franchises/{your-slug}` for sport rosters. Protected requests include the bearer token. The app never connects directly to PostgreSQL and never contains `ADMIN_TOKEN`.

New managers can paste the website's existing `https://mosaicfantasy.com/join/{token}` invite URL into the in-app claim form. For local development, use an invite URL with the same host and scheme as `API_BASE_URL` (for example `http://localhost:8090/join/{token}`). Logging out revokes that mobile token; it does not sign the website out.

## Remaining chunks

Roster-changing team actions; league play and live feeds; research and commissioner workflows; account deletion; a separate demo API; App Store assets, testing, and submission. Website cookie sign-in remains unchanged.
