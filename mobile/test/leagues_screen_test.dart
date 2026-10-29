// Verifies the Leagues landing flow, pending-league safety, invite joining, and drawer navigation.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';
import 'package:mosaic_fantasy/main.dart';

class TestTokenStore implements TokenStore {
  TestTokenStore([this.value]);
  String? value;
  @override
  Future<String?> read() async => value;
  @override
  Future<void> write(String token) async => value = token;
  @override
  Future<void> delete() async => value = null;
}

void main() {
  testWidgets('unverified account can resend email but not claim an invite', (
    tester,
  ) async {
    var resent = false;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":false}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response('[]', 200);
          case '/api/accounts/verification':
            resent = true;
            return http.Response('', 204);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('new@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Join a league'), findsNothing);
    expect(find.text('Verify your email'), findsOneWidget);
    await tester.tap(find.text('Resend verification email'));
    await tester.pumpAndSettle();
    expect(resent, isTrue);
  });

  testWidgets('restored account lands on Leagues and can sign out on iPad', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1024, 1366);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/refresh':
            return http.Response(
              '{"token":"fresh","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response('[]', 200);
          case '/api/mobile/auth/logout':
            return http.Response('', 204);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final store = TestTokenStore('old');
    final session = SessionController(api: api, store: store);
    await session.restore();
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Leagues'), findsOneWidget);
    expect(find.text('Mosaic Fantasy'), findsNothing);
    expect(find.byType(AppBar), findsNothing);
    expect(find.text('No leagues yet'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.menu));
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<ListTile>(find.widgetWithText(ListTile, 'Matchups'))
          .enabled,
      isFalse,
    );
    await tester.tap(find.text('new@example.com'));
    await tester.pumpAndSettle();
    expect(find.text('Account'), findsOneWidget);
    await tester.ensureVisible(find.text('Sign out'));
    await tester.tap(find.text('Sign out'));
    await tester.pumpAndSettle();
    expect(find.text('Welcome back'), findsOneWidget);
    expect(store.value, isNull);
  });

  testWidgets('hides pending setups and blocks unrelated gameplay', (
    tester,
  ) async {
    var dynastyCalls = 0;
    var standingsCalls = 0;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","slug":"mine","name":"Mine","email":"ada@example.com"}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              '[{"id":"pending","name":"Future League","franchise_id":"mine","franchise_name":"Mine","sports":["nhl"],"ready":false},{"id":"other","name":"Other League","franchise_id":"mine","franchise_name":"Mine","sports":["nba"],"ready":true}]',
              200,
            );
          case '/api/dynasty':
            dynastyCalls++;
            return http.Response(
              '{"id":"legacy","name":"Legacy","leagues":[],"franchises":[]}',
              200,
            );
          case '/api/standings':
            standingsCalls++;
            return http.Response('[]', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Future League'), findsNothing);
    expect(find.textContaining('Setup pending'), findsNothing);
    expect(find.byIcon(Icons.arrow_forward), findsNothing);
    expect(dynastyCalls, 0);
    await tester.tap(find.text('Other League'));
    await tester.pumpAndSettle();
    expect(dynastyCalls, 1);
    expect(standingsCalls, 0);
    expect(
      find.textContaining('mobile gameplay is not available yet'),
      findsOneWidget,
    );
  });

  testWidgets('a pending-only account keeps the invite path available', (
    tester,
  ) async {
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"ada@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              '[{"id":"pending","name":"Future League","franchise_id":"mine","franchise_name":"Mine","sports":["nhl"],"ready":false}]',
              200,
            );
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Future League'), findsNothing);
    expect(find.text('Setup pending'), findsNothing);
    expect(find.text('No leagues yet'), findsOneWidget);
    expect(find.text('Join a league'), findsOneWidget);
  });

  testWidgets('verified account can join by commissioner invite', (
    tester,
  ) async {
    var joined = false;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              joined
                  ? '[{"id":"legacy","name":"Joined League","franchise_id":"mine","franchise_name":"Mine","sports":["nhl"],"ready":true}]'
                  : '[]',
              200,
            );
          case '/api/mobile/invites/abc/accept':
            joined = true;
            return http.Response(
              '{"token":"joined","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","slug":"mine","name":"Mine","email":"new@example.com","verified":true}}',
              200,
            );
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('new@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Join a league'));
    await tester.pumpAndSettle();
    expect(
      find.text('Paste the link a commissioner sent you.'),
      findsOneWidget,
    );
    await tester.enterText(
      find.byKey(const Key('invite-link-field')),
      'https://example.test/join/abc',
    );
    await tester.tap(find.text('Join league'));
    await tester.pumpAndSettle();
    expect(joined, isTrue);
    expect(find.text('Joined League'), findsOneWidget);
  });

  testWidgets('playable card uses website sport colors and opens its league', (
    tester,
  ) async {
    var dynastyCalls = 0;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","email":"ada@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              '[{"id":"legacy","name":"Mosaic Dynasty","franchise_id":"mine","franchise_name":"Northern Lights","sports":["cbb","nba","wnba","nhl","nfl","mlb"],"ready":true}]',
              200,
            );
          case '/api/dynasty':
            dynastyCalls++;
            return http.Response(
              '{"id":"legacy","name":"Mosaic Dynasty","leagues":[],"franchises":[]}',
              200,
            );
          case '/api/standings':
          case '/api/overall':
          case '/api/activity':
            return http.Response('[]', 200);
          case '/api/odds':
            return http.Response('{}', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Leagues'), findsOneWidget);
    expect(find.text('Ready to play'), findsNothing);
    expect(find.text('1 league'), findsNothing);
    expect(find.byIcon(Icons.arrow_forward), findsNothing);
    expect(find.text('Northern Lights'), findsOneWidget);
    const sportColors = <String, Color>{
      'cbb': Color(0xffff9547),
      'nba': Color(0xffa48bff),
      'wnba': Color(0xffff8fb3),
      'nhl': Color(0xff4fd1e0),
      'nfl': Color(0xff62d27a),
      'mlb': Color(0xffff7676),
    };
    for (final entry in sportColors.entries) {
      expect(find.text(entry.key.toUpperCase()), findsOneWidget);
      final dot = tester.widget<DecoratedBox>(
        find.byKey(Key('sport-dot-${entry.key}')),
      );
      expect((dot.decoration as BoxDecoration).color, entry.value);
    }
    await tester.tap(find.text('Mosaic Dynasty'));
    await tester.pumpAndSettle();
    expect(dynastyCalls, 1);
    expect(find.text('Standings'), findsOneWidget);
  });

  testWidgets('load error has recovery action', (tester) async {
    var calls = 0;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"ada@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            calls++;
            return calls == 1
                ? http.Response('{"error":"Temporary outage"}', 503)
                : http.Response('[]', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Could not load your leagues'), findsOneWidget);
    expect(find.text('Temporary outage'), findsOneWidget);
    await tester.tap(find.text('Try again'));
    await tester.pumpAndSettle();
    expect(find.text('No leagues yet'), findsOneWidget);
  });

  testWidgets('large text keeps league and invite controls readable', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"ada@example.com","verified":true}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response('[]', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: TestTokenStore());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(
      MediaQuery(
        data: const MediaQueryData(textScaler: TextScaler.linear(2)),
        child: MosaicApp(api: api, session: session),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Leagues'), findsOneWidget);
    await tester.drag(find.byType(ListView).first, const Offset(0, -550));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Join a league'));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.byKey(const Key('invite-link-field')), findsOneWidget);
  });
}
