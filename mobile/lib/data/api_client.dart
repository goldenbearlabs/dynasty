// Calls the Go API for public data and secure mobile account sessions.

import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

class ApiException implements Exception {
  const ApiException(this.message, {this.statusCode});
  final String message;
  final int? statusCode;

  @override
  String toString() => message;
}

class ApiClient {
  ApiClient({required String baseUrl, http.Client? httpClient})
    : baseUri = Uri.parse(baseUrl.endsWith('/') ? baseUrl : '$baseUrl/'),
      client = httpClient ?? http.Client();

  final Uri baseUri;
  final http.Client client;
  String? Function()? tokenProvider;

  Future<Object?> request(
    String method,
    String path, {
    Map<String, String>? query,
    Map<String, dynamic>? body,
    String? token,
  }) async {
    final uri = baseUri.resolve(path).replace(queryParameters: query);
    final authToken = token ?? tokenProvider?.call();
    try {
      final response = await client
          .send(
            http.Request(method, uri)
              ..headers.addAll({
                if (body != null) 'Content-Type': 'application/json',
                if (authToken != null) 'Authorization': 'Bearer $authToken',
              })
              ..body = body == null ? '' : jsonEncode(body),
          )
          .timeout(const Duration(seconds: 15));
      final full = await http.Response.fromStream(response);
      if (full.statusCode < 200 || full.statusCode >= 300) {
        String? serverMessage;
        try {
          serverMessage = asJson(jsonDecode(full.body))['error'] as String?;
        } catch (_) {}
        throw ApiException(
          serverMessage ??
              'The server returned ${full.statusCode}. Please try again.',
          statusCode: full.statusCode,
        );
      }
      return full.body.isEmpty ? null : jsonDecode(full.body);
    } on ApiException {
      rethrow;
    } catch (_) {
      throw const ApiException(
        'Could not load data. Check your connection and API address.',
      );
    }
  }

  Future<Json> get(String path, [Map<String, String>? query]) async =>
      asJson(await request('GET', path, query: query));

  Future<Dynasty> dynasty() async => Dynasty.fromJson(await get('api/dynasty'));
  Future<List<Json>> competitions() async =>
      readList(await request('GET', 'api/competitions'), (json) => json);
  Future<List<MyLeague>> myLeagues() async =>
      readList(await request('GET', 'api/my-leagues'), MyLeague.fromJson);
  Future<List<StandingsTable>> standings() async =>
      readList(await request('GET', 'api/standings'), StandingsTable.fromJson);
  Future<List<OverallYear>> overall() async =>
      readList(await request('GET', 'api/overall'), OverallYear.fromJson);
  Future<LeagueOdds> odds() async => LeagueOdds.fromJson(await get('api/odds'));
  Future<List<ActivityItem>> activity() async =>
      readList(await request('GET', 'api/activity'), ActivityItem.fromJson);
  Future<FranchiseDetail> franchise(String slug) async =>
      FranchiseDetail.fromJson(
        await get('api/franchises/${Uri.encodeComponent(slug)}'),
      );
  Future<PlayerPage> players({
    String? competition,
    String? query,
    int page = 1,
  }) async => PlayerPage.fromJson(
    await get('api/players', {
      if (competition != null && competition.isNotEmpty)
        'competition': competition,
      if (query != null && query.isNotEmpty) 'q': query,
      'page': '$page',
    }),
  );
  Future<Scoreboard> scores(String competition, {String? day}) async =>
      Scoreboard.fromJson(
        await get(
          'api/scores/${Uri.encodeComponent(competition)}',
          day == null ? null : {'day': day},
        ),
      );

  Future<MobileAuth> mobileLogin(String email, String password) async =>
      MobileAuth.fromJson(
        asJson(
          await request(
            'POST',
            'api/mobile/auth/login',
            body: {'email': email, 'password': password},
          ),
        ),
      );

  Future<bool> registerAccount(String email, String password) async =>
      (asJson(
            await request(
              'POST',
              'api/accounts',
              body: {'email': email, 'password': password},
            ),
          )['email_sent']
          as bool?) ??
      false;

  Future<void> resendVerification(String email) async {
    await request('POST', 'api/accounts/verification', body: {'email': email});
  }

  Future<void> requestPasswordReset(String email) async {
    await request(
      'POST',
      'api/accounts/password-reset',
      body: {'email': email},
    );
  }

  Future<MobileAuth> mobileClaim(
    String invite,
    String email,
    String password,
  ) async => MobileAuth.fromJson(
    asJson(
      await request(
        'POST',
        'api/mobile/invites/${Uri.encodeComponent(invite)}/claim',
        body: {'email': email, 'password': password},
      ),
    ),
  );

  Future<MobileAuth> mobileAccept(String invite, String token) async =>
      MobileAuth.fromJson(
        asJson(
          await request(
            'POST',
            'api/mobile/invites/${Uri.encodeComponent(invite)}/accept',
            token: token,
          ),
        ),
      );

  Future<SessionInfo> mobileSession(String token) async => SessionInfo.fromJson(
    asJson(await request('GET', 'api/mobile/session', token: token)),
  );

  Future<MobileAuth> mobileRefresh(String token) async => MobileAuth.fromJson(
    asJson(await request('POST', 'api/mobile/auth/refresh', token: token)),
  );

  Future<void> mobileLogout(String token) async {
    await request('POST', 'api/mobile/auth/logout', token: token);
  }

  Future<void> changePassword(String current, String next) async {
    await request(
      'POST',
      'api/account/password',
      body: {'current_password': current, 'new_password': next},
    );
  }

  Future<void> beginEmailChange(String current, String email) async {
    await request(
      'POST',
      'api/account/email-change',
      body: {'current_password': current, 'new_email': email},
    );
  }

  Future<void> confirmEmailChange(String code) async {
    await request(
      'POST',
      'api/account/email-change/confirm',
      body: {'code': code},
    );
  }

  Future<List<Json>> deletionOptions() async => readList(
    await request('GET', 'api/account/deletion-options'),
    (json) => json,
  );

  Future<void> deleteAccount(
    String password,
    Map<String, String> transfers,
  ) async {
    await request(
      'DELETE',
      'api/account',
      body: {'password': password, 'commissioner_transfers': transfers},
    );
  }

  Future<void> setFranchiseIdentity(
    String franchiseId,
    String name,
    String imageUrl,
  ) async {
    await request(
      'PUT',
      'api/franchises/${Uri.encodeComponent(franchiseId)}/identity',
      body: {'name': name, 'image_url': imageUrl},
    );
  }

  Future<void> setTeamIdentity(
    String franchiseId,
    String leagueId,
    String name,
    String imageUrl,
  ) async {
    await request(
      'PUT',
      'api/franchises/${Uri.encodeComponent(franchiseId)}/leagues/${Uri.encodeComponent(leagueId)}/identity',
      body: {'name': name, 'image_url': imageUrl},
    );
  }
}
