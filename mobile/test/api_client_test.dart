// Verifies public API URL construction, JSON parsing, and clear HTTP errors.

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';

void main() {
  test('searches players using the chosen API base URL', () async {
    final api = ApiClient(
      baseUrl: 'http://localhost:8090',
      httpClient: MockClient((request) async {
        expect(request.url.host, 'localhost');
        expect(request.url.path, '/api/players');
        expect(request.url.queryParameters['competition'], 'nhl');
        expect(request.url.queryParameters['q'], 'McDavid');
        return http.Response(
          '{"players":[{"full_name":"Connor McDavid","competition":"nhl","positions":["C"],"team_abbrev":"EDM","owner_name":""}],"total":1,"page":1,"per_page":50}',
          200,
        );
      }),
    );
    final page = await api.players(competition: 'nhl', query: 'McDavid');
    expect(page.total, 1);
    expect(page.players.single.name, 'Connor McDavid');
  });

  test('reports a server error without hiding the status', () async {
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((_) async => http.Response('unavailable', 503)),
    );
    expect(
      () => api.dynasty(),
      throwsA(
        isA<ApiException>().having(
          (error) => error.message,
          'message',
          contains('503'),
        ),
      ),
    );
  });

  test(
    'loads account leagues and league dashboard data with a mobile token',
    () async {
      final api = ApiClient(
        baseUrl: 'https://example.test',
        httpClient: MockClient((request) async {
          expect(request.headers['Authorization'], 'Bearer mobile-token');
          switch (request.url.path) {
            case '/api/my-leagues':
              return http.Response(
                '[{"id":"legacy","name":"Mosaic","franchise_id":"team","franchise_name":"Ice Kings","sports":["nhl"],"ready":true},{"id":"saved","name":"Future","franchise_id":"future-team","franchise_name":"Future Team","sports":["nba"],"ready":false}]',
                200,
              );
            case '/api/standings':
              return http.Response(
                '[{"league_id":"nhl","competition":"nhl","format":"head_to_head","season":{"status":"active"},"rows":[{"franchise_id":"team","points_for":12.5,"wins":1}]}]',
                200,
              );
            case '/api/activity':
              return http.Response(
                '[{"id":7,"kind":"add","competition":"nhl","franchise_name":"Ice Kings","player_name":"A Player","created_at":"2026-10-23T12:00:00Z","detail":{"list":"main"}}]',
                200,
              );
            case '/api/overall':
              return http.Response(
                '[{"year":2026,"final":false,"rows":[{"franchise_id":"team","points":4,"adjusted_points":9.5}]}]',
                200,
              );
            case '/api/odds':
              return http.Response(
                '{"leagues":[{"competition":"nhl","rows":[{"franchise_id":"team","playoffs":0.75}]}],"overall":[{"franchise_id":"team","title":0.25}]}',
                200,
              );
            default:
              fail('Unexpected ${request.url.path}');
          }
        }),
      )..tokenProvider = () => 'mobile-token';

      final leagues = await api.myLeagues();
      expect(leagues.first.franchiseId, 'team');
      expect(leagues.last.ready, isFalse);
      final standings = await api.standings();
      expect(standings.single.rows.single.pointsFor, 12.5);
      final activity = await api.activity();
      expect(activity.single.playerName, 'A Player');
      final overall = await api.overall();
      expect(overall.single.rows.single.adjustedPoints, 9.5);
      final odds = await api.odds();
      expect(odds.leagues.single.rows.single.playoffs, 0.75);
      expect(odds.overall.single.title, 0.25);
    },
  );
}
