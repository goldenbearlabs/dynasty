// Checks League dashboard filtering, refresh, and recovery from API errors.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/models.dart';
import 'package:mosaic_fantasy/screens/league_dashboard_screen.dart';

const dynasty = Dynasty(
  id: 'legacy',
  name: 'Mosaic Fantasy',
  leagues: [League(id: 'nhl', name: 'Hockey', competition: 'nhl')],
  franchises: [
    Franchise(
      id: 'mine',
      name: 'My Franchise',
      managerName: 'Ada',
      slug: 'mine',
    ),
  ],
);
const league = MyLeague(
  id: 'legacy',
  name: 'Mosaic Fantasy',
  franchiseId: 'mine',
  franchiseName: 'My Franchise',
  sports: ['nhl'],
  ready: true,
);

void main() {
  testWidgets('filters activity and reloads standings on pull-to-refresh', (
    tester,
  ) async {
    var loads = 0;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/standings':
            loads++;
            return http.Response(
              '[{"league_id":"nhl","competition":"nhl","format":"total_points","season":{"status":"active"},"rows":[{"franchise_id":"mine","points":12}]}]',
              200,
            );
          case '/api/overall':
            return http.Response('[]', 200);
          case '/api/activity':
            return http.Response(
              '[{"id":1,"kind":"add","competition":"nhl","franchise_name":"My Franchise","player_name":"Player One"}]',
              200,
            );
          case '/api/odds':
            return http.Response('unavailable', 503);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: LeagueDashboardScreen(
            api: api,
            dynasty: dynasty,
            league: league,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('My Franchise'), findsOneWidget);
    expect(find.text('My Franchise added Player One'), findsOneWidget);
    await tester.ensureVisible(find.widgetWithText(ChoiceChip, 'Trades'));
    await tester.tap(find.widgetWithText(ChoiceChip, 'Trades'));
    await tester.pumpAndSettle();
    expect(find.text('No activity matches these filters.'), findsOneWidget);
    await tester.tap(find.text('Clear filters'));
    await tester.pumpAndSettle();
    expect(find.text('My Franchise added Player One'), findsOneWidget);
    final refreshing = tester
        .state<RefreshIndicatorState>(find.byType(RefreshIndicator))
        .show();
    await tester.pumpAndSettle();
    await refreshing;
    expect(loads, 2);
  });

  testWidgets('shows an API error and retries', (tester) async {
    var attempts = 0;
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        switch (request.url.path) {
          case '/api/standings':
            attempts++;
            return attempts == 1
                ? http.Response('unavailable', 503)
                : http.Response('[]', 200);
          case '/api/overall':
          case '/api/activity':
            return http.Response('[]', 200);
          case '/api/odds':
            return http.Response('{"leagues":[],"overall":[]}', 200);
          default:
            fail('Unexpected ${request.url.path}');
        }
      }),
    );
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: LeagueDashboardScreen(
            api: api,
            dynasty: dynasty,
            league: league,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.textContaining('503'), findsOneWidget);
    await tester.tap(find.text('Try again'));
    await tester.pumpAndSettle();
    expect(find.text('No standings yet.'), findsOneWidget);
    expect(attempts, 2);
  });
}
