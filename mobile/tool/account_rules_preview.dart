// Shows Account or Rules with sample data for iPhone/iPad layout review, without credentials or a server.

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:mosaic_fantasy/data/api_client.dart';
import 'package:mosaic_fantasy/data/session_controller.dart';
import 'package:mosaic_fantasy/screens/account_screen.dart';
import 'package:mosaic_fantasy/screens/rules_screen.dart';

const page = String.fromEnvironment('PREVIEW_PAGE', defaultValue: 'account');

class _Store implements TokenStore {
  String? value;
  @override Future<String?> read() async => value;
  @override Future<void> write(String token) async => value = token;
  @override Future<void> delete() async => value = null;
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final api = ApiClient(baseUrl: 'https://preview.invalid', httpClient: MockClient((request) async {
    switch (request.url.path) {
      case '/api/mobile/auth/login':
        return http.Response('{"token":"preview","expires_at":"2030-01-01T00:00:00Z","session":{"id":"mine","name":"Northern Lights","manager_name":"Ethan Carter","slug":"north","email":"ethan@example.com","verified":true}}', 200);
      case '/api/my-leagues':
        return http.Response('[{"id":"dynasty","name":"Mosaic Dynasty","franchise_id":"mine","franchise_name":"Northern Lights","sports":["nhl","nba","nfl"],"ready":true}]', 200);
      case '/api/dynasty':
        return http.Response('{"id":"dynasty","name":"Mosaic Dynasty","settings":{"overall_title":{"enabled":true,"points_by_finish":[16,14,12]}},"franchises":[{"id":"mine","name":"Northern Lights","manager_name":"Ethan Carter","slug":"north"}],"team_identities":[{"franchise_id":"mine","league_id":"nhl-id","name":"Ice Kings"}],"leagues":[{"id":"nhl-id","name":"Hockey","competition":"nhl","settings":{"roster":{"main":19,"reserve":6,"reserve_eligibility":"anyone"},"lineup":{"period":"day","lock":"game_start","slots":[{"name":"Forward","count":3,"positions":["C","LW","RW"]}]},"scoring":{"goals":3},"format":{"type":"head_to_head","matchup_days":7,"playoff_teams":8,"reseed":false},"waivers":{"mode":"order","hours":24},"draft":{"rounds":3}}},{"id":"nba-id","name":"Basketball","competition":"nba","settings":{"roster":{"main":12,"reserve":4}}}]}', 200);
      case '/api/competitions':
        return http.Response('[{"key":"nhl","name":"Hockey","stats":[{"key":"goals","label":"Goals"}],"defaults":{"roster":{"main":19,"reserve":6}}},{"key":"nba","name":"Basketball","stats":[],"defaults":{"roster":{"main":12,"reserve":4}}}]', 200);
      default:
        return http.Response('{"error":"Preview action unavailable"}', 404);
    }
  }));
  final session = SessionController(api: api, store: _Store());
  await session.restore();
  await session.signIn('ethan@example.com', 'preview');
  runApp(MaterialApp(
    debugShowCheckedModeBanner: false,
    theme: ThemeData(
      brightness: Brightness.dark,
      colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xff8499ff), brightness: Brightness.dark, surface: const Color(0xff141822)),
      scaffoldBackgroundColor: const Color(0xff0b0e15),
      cardColor: const Color(0xff141822),
      useMaterial3: true,
    ),
    home: Scaffold(
      appBar: AppBar(title: Text(page == 'rules' ? 'Rules' : 'Account')),
      body: SafeArea(child: page == 'rules'
          ? RulesScreen(api: api)
          : AccountScreen(api: api, session: session, onSignOut: () async {})),
    ),
  ));
}
