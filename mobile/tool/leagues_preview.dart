// Runs the real Leagues screen with sample data for safe iPhone/iPad visual review.
// No server, credentials, or production league data are used by this preview.

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';
import 'package:mosaic_fantasy/main.dart' as app;

class _PreviewTokenStore implements TokenStore {
  String? token;

  @override
  Future<String?> read() async => token;

  @override
  Future<void> write(String value) async => token = value;

  @override
  Future<void> delete() async => token = null;
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(await buildPreviewApp());
}

Future<Widget> buildPreviewApp() async {
  final api = ApiClient(
    baseUrl: 'https://preview.invalid',
    httpClient: MockClient((request) async {
      switch (request.url.path) {
        case '/api/mobile/auth/login':
        case '/api/mobile/auth/refresh':
          return http.Response(
            '{"token":"preview","expires_at":"2030-01-01T00:00:00Z","session":{"id":"preview-manager","email":"preview@example.com","verified":true}}',
            200,
          );
        case '/api/my-leagues':
          return http.Response(
            '[{"id":"preview-hockey","name":"Mosaic Dynasty","franchise_id":"preview-manager","franchise_name":"Northern Lights","sports":["nhl","nba","nfl","mlb","wnba","cbb"],"ready":true},{"id":"preview-football","name":"Sunday League","franchise_id":"preview-manager","franchise_name":"Edmonton Owls","sports":["nfl","nba"],"ready":true},{"id":"preview-next","name":"Next Season","franchise_id":"preview-manager","franchise_name":"Northern Lights","sports":["nhl","mlb"],"ready":false}]',
            200,
          );
        case '/api/mobile/auth/logout':
          return http.Response('', 204);
        case '/api/dynasty':
          return http.Response(
            '{"id":"preview-hockey","name":"Mosaic Dynasty","leagues":[],"franchises":[]}',
            200,
          );
        case '/api/standings':
        case '/api/overall':
        case '/api/activity':
          return http.Response('[]', 200);
        case '/api/odds':
          return http.Response('{}', 200);
        default:
          return http.Response('{"error":"Preview action unavailable"}', 404);
      }
    }),
  );
  final session = SessionController(api: api, store: _PreviewTokenStore());
  await session.restore();
  await session.signIn('preview@example.com', 'preview');
  return app.MosaicApp(api: api, session: session);
}
