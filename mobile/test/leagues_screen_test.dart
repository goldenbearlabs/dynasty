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
    expect(find.text('Join league'), findsNothing);
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
    expect(find.text('Choose a league'), findsOneWidget);
    expect(find.text('You have not joined a league yet.'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.menu));
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<ListTile>(find.widgetWithText(ListTile, 'Matchups'))
          .enabled,
      isFalse,
    );
    await tester.tap(find.text('Sign out'));
    await tester.pumpAndSettle();
    expect(find.text('Welcome back'), findsOneWidget);
    expect(store.value, isNull);
  });

  testWidgets('pending and unrelated leagues never open legacy gameplay', (
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
    expect(find.textContaining('Setup pending'), findsOneWidget);
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
    await tester.enterText(
      find.byType(TextField),
      'https://example.test/join/abc',
    );
    await tester.tap(find.text('Join league'));
    await tester.pumpAndSettle();
    expect(joined, isTrue);
    expect(find.text('Joined League'), findsOneWidget);
  });
}
