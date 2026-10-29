// Verifies recommended rules, live settings, and rejection of an unrelated gameplay dynasty.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/models.dart';
import 'package:mosaic_fantasy/screens/rules_screen.dart';

void main() {
  const chosen = MyLeague(
    id: 'league-id',
    name: 'The League',
    franchiseId: 'mine',
    franchiseName: 'Mine',
    sports: ['nhl'],
    ready: true,
  );
  testWidgets(
    'no selected league shows public recommended rules without requesting a dynasty',
    (tester) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final api = ApiClient(
        baseUrl: 'https://example.test',
        httpClient: MockClient((request) async {
          expect(request.url.path, '/api/competitions');
          return http.Response(
            '[{"key":"nhl","name":"Hockey","defaults":{"roster":{"main":18,"reserve":6},"format":{"type":"head_to_head","playoff_teams":8,"reseed":false}}}]',
            200,
          );
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: RulesScreen(api: api)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Everywhere'), findsOneWidget);
      expect(find.text('How it was balanced'), findsOneWidget);
      await tester.tap(find.text('Hockey'));
      await tester.pumpAndSettle();
      expect(
        find.textContaining('Recommended starting settings'),
        findsOneWidget,
      );
      expect(find.text('18 players · 0 starters'), findsOneWidget);
      await tester.ensureVisible(find.text('How it was balanced'));
      await tester.tap(find.text('How it was balanced'));
      await tester.pumpAndSettle();
      expect(find.text('What is still uneven'), findsOneWidget);
    },
  );

  testWidgets(
    'selected league uses its current settings and blocks a mismatched dynasty',
    (tester) async {
      var matches = true;
      final api = ApiClient(
        baseUrl: 'https://example.test',
        httpClient: MockClient((request) async {
          if (request.url.path == '/api/competitions') {
            return http.Response(
              '[{"key":"nhl","name":"Hockey","stats":[],"defaults":{"roster":{"main":18}}}]',
              200,
            );
          }
          if (request.url.path == '/api/dynasty') {
            return http.Response(
              '{"id":"${matches ? 'league-id' : 'other-id'}","name":"The League","franchises":[],"leagues":[{"id":"nhl-id","name":"Hockey","competition":"nhl","settings":{"roster":{"main":21,"reserve":5},"format":{"type":"head_to_head","reseed":false}}}]}',
              200,
            );
          }
          fail('Unexpected ${request.url.path}');
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: RulesScreen(api: api, selectedLeague: chosen),
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('Hockey'));
      await tester.pumpAndSettle();
      expect(find.text('21 players · 0 starters'), findsOneWidget);
      matches = false;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: RulesScreen(
              key: const ValueKey('mismatch'),
              api: api,
              selectedLeague: chosen,
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.textContaining('not available through this gameplay API'),
        findsOneWidget,
      );
    },
  );
}
