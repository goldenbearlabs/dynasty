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
}
