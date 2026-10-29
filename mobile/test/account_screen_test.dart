// Checks account controls and ensures team identity cannot cross the selected dynasty boundary.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/models.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';
import 'package:mosaic_fantasy/screens/account_screen.dart';

class _Store implements TokenStore {
  String? token;
  @override
  Future<String?> read() async => token;
  @override
  Future<void> write(String value) async => token = value;
  @override
  Future<void> delete() async => token = null;
}

void main() {
  testWidgets(
    'an account without a league can change password and loses its old session',
    (tester) async {
      var changed = false;
      final api = ApiClient(
        baseUrl: 'https://example.test',
        httpClient: MockClient((request) async {
          switch (request.url.path) {
            case '/api/mobile/auth/login':
              return http.Response(
                '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":false}}',
                200,
              );
            case '/api/account/password':
              changed = request.body.contains('new secret password');
              return http.Response('', 204);
            default:
              fail('Unexpected ${request.url.path}');
          }
        }),
      );
      final session = SessionController(api: api, store: _Store());
      await session.restore();
      await session.signIn('new@example.com', 'old password');
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: AccountScreen(
              api: api,
              session: session,
              onSignOut: () async {},
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.text(
          'Join a playable league to set your franchise and team identities.',
        ),
        findsOneWidget,
      );
      expect(find.text('Email not verified'), findsOneWidget);
      await tester.ensureVisible(find.text('Change password'));
      await tester.enterText(
        find.widgetWithText(TextField, 'Current password').last,
        'old password',
      );
      await tester.enterText(
        find.widgetWithText(TextField, 'New password'),
        'new secret password',
      );
      await tester.tap(find.text('Change password'));
      await tester.pumpAndSettle();
      expect(changed, isTrue);
      expect(session.session, isNull);
    },
  );

  testWidgets('only the matching playable dynasty exposes team identity', (
    tester,
  ) async {
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","name":"Northern Lights","manager_name":"Ada","slug":"north","email":"ada@example.com"}}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              '[{"id":"different","name":"Other League","franchise_id":"mine","franchise_name":"Northern Lights","sports":["nhl"],"ready":true}]',
              200,
            );
          case '/api/dynasty':
            return http.Response(
              '{"id":"legacy","name":"Legacy","franchises":[{"id":"mine","name":"Northern Lights"}],"leagues":[{"id":"nhl","name":"Hockey","competition":"nhl"}]}',
              200,
            );
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(api: api, store: _Store());
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    const chosen = MyLeague(
      id: 'different',
      name: 'Other League',
      franchiseId: 'mine',
      franchiseName: 'Northern Lights',
      sports: ['nhl'],
      ready: true,
    );
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: AccountScreen(
            api: api,
            session: session,
            selectedLeague: chosen,
            onSignOut: () async {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(
      find.textContaining('not available for this league'),
      findsOneWidget,
    );
    expect(find.text('Save identity'), findsNothing);
  });

  testWidgets(
    'a matching league edits its sport team through the website identity API',
    (tester) async {
      String? savedName;
      final api = ApiClient(
        baseUrl: 'https://example.test',
        httpClient: MockClient((request) async {
          switch (request.url.path) {
            case '/api/mobile/auth/login':
              return http.Response(
                '{"token":"token","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","name":"Northern Lights","manager_name":"Ada","slug":"north","email":"ada@example.com"}}',
                200,
              );
            case '/api/my-leagues':
              return http.Response(
                '[{"id":"legacy","name":"Mosaic Dynasty","franchise_id":"mine","franchise_name":"Northern Lights","sports":["nhl"],"ready":true}]',
                200,
              );
            case '/api/dynasty':
              return http.Response(
                '{"id":"legacy","name":"Mosaic Dynasty","franchises":[{"id":"mine","name":"Northern Lights"}],"leagues":[{"id":"nhl","name":"Hockey","competition":"nhl"}],"team_identities":[]}',
                200,
              );
            case '/api/franchises/mine/leagues/nhl/identity':
              savedName =
                  (request.body.isNotEmpty ? request.body : '').contains(
                    'Ice Kings',
                  )
                  ? 'Ice Kings'
                  : '';
              return http.Response('', 204);
            default:
              fail('Unexpected ${request.url.path}');
          }
        }),
      );
      final session = SessionController(api: api, store: _Store());
      await session.restore();
      await session.signIn('ada@example.com', 'password');
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: AccountScreen(
              api: api,
              session: session,
              onSignOut: () async {},
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Hockey'), findsOneWidget);
      await tester.ensureVisible(find.widgetWithText(TextField, 'Team name'));
      await tester.enterText(
        find.widgetWithText(TextField, 'Team name'),
        'Ice Kings',
      );
      await tester.ensureVisible(find.text('Save identity').last);
      await tester.tap(find.text('Save identity').last);
      await tester.pumpAndSettle();
      expect(savedName, 'Ice Kings');
    },
  );
}
