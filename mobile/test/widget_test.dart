// Checks signed-out entry and the authenticated app shell on iPhone and iPad.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';
import 'package:mosaic_fantasy/main.dart';

class MemoryTokenStore implements TokenStore {
  String? value;
  @override
  Future<String?> read() async => value;
  @override
  Future<void> write(String token) async {
    value = token;
  }

  @override
  Future<void> delete() async {
    value = null;
  }
}

void main() {
  final api = ApiClient(
    baseUrl: 'https://example.test',
    httpClient: MockClient((request) async {
      expect(request.url.path, '/api/dynasty');
      return http.Response(
        '{"name":"Test Dynasty","leagues":[{"name":"Hockey","competition":"nhl"}],"franchises":[]}',
        200,
      );
    }),
  );

  testWidgets('shows sign-in instead of private league data on iPhone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final session = SessionController(api: api, store: MemoryTokenStore());
    await session.restore();
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Welcome back'), findsOneWidget);
    expect(find.text('Test Dynasty'), findsNothing);
    expect(find.byType(NavigationBar), findsNothing);
  });

  testWidgets('does not show league navigation when signed out on iPad', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1024, 1366);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final session = SessionController(api: api, store: MemoryTokenStore());
    await session.restore();
    await tester.pumpWidget(MosaicApp(api: api, session: session));
    await tester.pumpAndSettle();
    expect(find.byType(NavigationRail), findsNothing);
    expect(find.byType(NavigationBar), findsNothing);
  });

  testWidgets('opens the selected playable league and shows standings', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final privateApi = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/mobile/auth/login':
            return http.Response(
              '{"token":"abc","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","slug":"my-franchise","name":"My Franchise","manager_name":"Ada","email":"ada@example.com"}}',
              200,
            );
          case '/api/dynasty':
            return http.Response(
              '{"id":"dynasty-id","name":"Mosaic Fantasy","leagues":[{"id":"nhl-id","name":"Hockey","competition":"nhl","settings":{"roster":{"main":12,"reserve":6}}}],"franchises":[{"id":"mine","name":"My Franchise","slug":"my-franchise"},{"id":"other","name":"Other Franchise","slug":"other"}],"team_identities":[{"franchise_id":"mine","league_id":"nhl-id","name":"Ice Kings"}]}',
              200,
            );
          case '/api/my-leagues':
            return http.Response(
              '[{"id":"dynasty-id","name":"Ice League","franchise_id":"mine","franchise_name":"My Franchise","sports":["nhl"],"ready":true}]',
              200,
            );
          case '/api/standings':
            return http.Response(
              '[{"league_id":"nhl-id","competition":"nhl","format":"head_to_head","season":{"status":"active"},"rows":[{"franchise_id":"mine","points_for":42,"points_against":35,"wins":2,"losses":1,"ties":0}]}]',
              200,
            );
          case '/api/overall':
            return http.Response('[]', 200);
          case '/api/activity':
            return http.Response(
              '[{"id":1,"kind":"add","competition":"nhl","franchise_name":"My Franchise","player_name":"Hockey Player","created_at":"2026-10-22T12:00:00Z"}]',
              200,
            );
          case '/api/odds':
            return http.Response('{"leagues":[],"overall":[]}', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    final session = SessionController(
      api: privateApi,
      store: MemoryTokenStore(),
    );
    await session.restore();
    await session.signIn('ada@example.com', 'password');
    await tester.pumpWidget(MosaicApp(api: privateApi, session: session));
    await tester.pumpAndSettle();
    expect(find.text('Leagues'), findsOneWidget);
    expect(find.text('Ice League'), findsOneWidget);
    await tester.tap(find.text('Ice League'));
    await tester.pumpAndSettle();
    expect(find.text('Standings'), findsOneWidget);
    await tester.tap(find.widgetWithText(ChoiceChip, 'NHL').first);
    await tester.pumpAndSettle();
    expect(find.text('Ice Kings'), findsOneWidget);
    expect(find.text('Recent activity'), findsOneWidget);
  });
}
