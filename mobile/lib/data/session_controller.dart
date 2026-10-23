// Restores and manages the manager's revocable token in iOS secure storage.

import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'api_client.dart';
import 'models.dart';

abstract class TokenStore {
  Future<String?> read();
  Future<void> write(String token);
  Future<void> delete();
}

class SecureTokenStore implements TokenStore {
  const SecureTokenStore();
  static const storage = FlutterSecureStorage();
  static const key = 'mosaic_mobile_token';

  @override
  Future<String?> read() => storage.read(key: key);
  @override
  Future<void> write(String token) => storage.write(key: key, value: token);
  @override
  Future<void> delete() => storage.delete(key: key);
}

class SessionController extends ChangeNotifier {
  SessionController({required this.api, required this.store});
  final ApiClient api;
  final TokenStore store;

  SessionInfo? session;
  String? token;
  String? restoreError;
  bool ready = false;

  Future<void> restore() async {
    restoreError = null;
    try {
      token = await store.read();
      if (token != null) {
        final refreshed = await api.mobileRefresh(token!);
        await _accept(refreshed);
      }
    } on ApiException catch (error) {
      if (error.statusCode == 401) {
        await _clear();
      } else {
        restoreError = error.message;
      }
    } catch (_) {
      restoreError = 'Could not restore your session. Please try again.';
    } finally {
      ready = true;
      notifyListeners();
    }
  }

  Future<void> refresh() async {
    if (token == null) return;
    try {
      await _accept(await api.mobileRefresh(token!));
      restoreError = null;
    } on ApiException catch (error) {
      if (error.statusCode == 401) {
        await _clear();
      } else {
        restoreError = error.message;
      }
    } catch (_) {
      restoreError = 'Could not refresh your session. Please try again.';
    }
    notifyListeners();
  }

  Future<void> signIn(String email, String password) async {
    await _accept(await api.mobileLogin(email, password));
    notifyListeners();
  }

  Future<bool> register(String email, String password) async {
    final sent = await api.registerAccount(email, password);
    await signIn(email, password);
    return sent;
  }

  Future<void> claim(String inviteUrl, String email, String password) async {
    final inviteToken = parseInvite(inviteUrl);
    await _accept(await api.mobileClaim(inviteToken, email, password));
    notifyListeners();
  }

  Future<void> acceptInvite(String inviteUrl) async {
    if (token == null) throw const ApiException('Sign in first.');
    await _accept(await api.mobileAccept(parseInvite(inviteUrl), token!));
    notifyListeners();
  }

  String parseInvite(String inviteUrl) {
    final uri = Uri.tryParse(inviteUrl.trim());
    if (uri == null ||
        uri.host != api.baseUri.host ||
        uri.scheme != api.baseUri.scheme ||
        uri.pathSegments.length != 2 ||
        uri.pathSegments.first != 'join' ||
        uri.pathSegments.last.isEmpty) {
      throw const ApiException(
        'Paste an invite link for this Mosaic Fantasy server.',
      );
    }
    return uri.pathSegments.last;
  }

  Future<void> logout() async {
    if (token != null) await api.mobileLogout(token!);
    await _clear();
    notifyListeners();
  }

  Future<void> _accept(MobileAuth value) async {
    await store.write(value.token);
    token = value.token;
    session = value.session;
  }

  Future<void> _clear() async {
    token = null;
    session = null;
    await store.delete();
  }
}
