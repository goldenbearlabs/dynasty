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

  testWidgets('shows only the signed-in manager’s sport teams and live rules', (
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
              '{"name":"Mosaic Fantasy","leagues":[{"id":"nhl-id","name":"Hockey","competition":"nhl","settings":{"roster":{"main":12,"reserve":6}}}],"franchises":[{"id":"mine","name":"My Franchise","slug":"my-franchise"},{"id":"other","name":"Other Franchise","slug":"other"}]}',
              200,
            );
          case '/api/franchises/my-franchise':
            return http.Response(
              '{"franchise":{"id":"mine","name":"My Franchise","slug":"my-franchise","manager_name":"Ada"},"rosters":[{"league_id":"nhl-id","competition":"nhl","team_name":"Ice Kings","players":[{"full_name":"Hockey Player","list":"main","positions":["C"]}]}]}',
              200,
            );
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
    expect(find.text('Mosaic Fantasy'), findsOneWidget);
    expect(find.text('League overview'), findsOneWidget);
    expect(find.text('Ice Kings'), findsOneWidget);
    expect(find.text('Other Franchise'), findsNothing);
    await tester.tap(find.text('Ice Kings'));
    await tester.pumpAndSettle();
    expect(find.text('Hockey Player'), findsOneWidget);
  });
}
