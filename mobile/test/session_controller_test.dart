// Checks secure-token restoration, rotation, logout, and invite-link validation.

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';

class MemoryStore implements TokenStore {
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

const response =
    '{"token":"new-token","expires_at":"2030-01-01T00:00:00Z","session":{"id":"team-1","name":"My Franchise","manager_name":"Ada","slug":"my-franchise","email":"ada@example.com"}}';

void main() {
  test('restores and rotates a stored token, then logs out', () async {
    final store = MemoryStore()..value = 'old-token';
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        if (request.url.path == '/api/mobile/auth/refresh') {
          expect(request.headers['authorization'], 'Bearer old-token');
          return http.Response(response, 200);
        }
        expect(request.url.path, '/api/mobile/auth/logout');
        expect(request.headers['authorization'], 'Bearer new-token');
        return http.Response('', 204);
      }),
    );
    final session = SessionController(api: api, store: store);
    await session.restore();
    expect(session.ready, isTrue);
    expect(session.session?.franchise?.name, 'My Franchise');
    expect(store.value, 'new-token');
    await session.logout();
    expect(store.value, isNull);
    expect(session.session, isNull);
  });

  test('clears a revoked token during restoration', () async {
    final store = MemoryStore()..value = 'revoked';
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient(
        (_) async => http.Response('{"error":"expired"}', 401),
      ),
    );
    final session = SessionController(api: api, store: store);
    await session.restore();
    expect(store.value, isNull);
    expect(session.session, isNull);
  });

  test('claims only an invite URL from the selected server', () async {
    final store = MemoryStore();
    final api = ApiClient(
      baseUrl: 'https://example.test',
      httpClient: MockClient((request) async {
        expect(request.url.path, '/api/mobile/invites/abc123/claim');
        return http.Response(response, 200);
      }),
    );
    final session = SessionController(api: api, store: store);
    expect(
      () => session.claim(
        'https://other.test/join/abc123',
        'a@b.com',
        'password',
      ),
      throwsA(isA<ApiException>()),
    );
    await session.claim(
      'https://example.test/join/abc123',
      'a@b.com',
      'password',
    );
    expect(store.value, 'new-token');
  });
}
