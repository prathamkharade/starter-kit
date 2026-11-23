import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/api_client.dart';
import '../../../core/network/api_exception.dart';
import '../../../core/storage/local_cache.dart';

class User {
  const User({required this.id, this.email, this.name = '', this.role = 'user'});
  final String id;
  final String? email;
  final String name;
  final String role;

  factory User.fromJson(Map<String, dynamic> j) =>
      User(id: j['id'] as String, email: j['email'] as String?, name: j['name'] as String? ?? '', role: j['role'] as String? ?? 'user');
}

sealed class AuthState {
  const AuthState();
}

class AuthUnknown extends AuthState {
  const AuthUnknown();
}

class AuthSignedOut extends AuthState {
  const AuthSignedOut();
}

class AuthSignedIn extends AuthState {
  const AuthSignedIn(this.user);
  final User user;
}

/// Owns the session lifecycle. UI calls these methods and catches ApiException.
class AuthController extends Notifier<AuthState> {
  Dio get _dio => ref.read(dioProvider);
  TokenManager get _tokens => ref.read(tokenManagerProvider);

  @override
  AuthState build() {
    final sub = _tokens.onSessionExpired.listen((_) => state = const AuthSignedOut());
    ref.onDispose(sub.cancel);
    unawaited(_restore());
    return const AuthUnknown();
  }

  Future<void> _restore() async {
    try {
      final res = await _tokens.refreshSession();
      state = res == null ? const AuthSignedOut() : AuthSignedIn(User.fromJson(res['user'] as Map<String, dynamic>));
    } catch (_) {
      // Offline at launch: let the user in with cached data if we had a session before.
      final cached = LocalCache.getJson<Map<String, dynamic>>('me');
      state = cached == null ? const AuthSignedOut() : AuthSignedIn(User.fromJson(cached));
    }
  }

  Future<T> _call<T>(Future<T> Function() fn) async {
    try {
      return await fn();
    } catch (e) {
      throw ApiException.from(e);
    }
  }

  Future<void> _signedIn(Map<String, dynamic> body) async {
    await _tokens.save(body);
    final user = body['user'] as Map<String, dynamic>;
    await LocalCache.putJson('me', user);
    state = AuthSignedIn(User.fromJson(user));
  }

  // ---- passwordless ----
  Future<void> requestCode(String channel, String destination) =>
      _call(() => _dio.post<void>('/v1/auth/otp/request', data: {'channel': channel, 'destination': destination}));

  Future<void> verifyCode(String channel, String destination, String code) => _call(() async {
        final r = await _dio.post<Map<String, dynamic>>('/v1/auth/otp/verify',
            data: {'channel': channel, 'destination': destination, 'code': code});
        await _signedIn(r.data!);
      });

  // ---- password ----
  Future<void> signInWithPassword(String email, String password) => _call(() async {
        final r = await _dio.post<Map<String, dynamic>>('/v1/auth/login', data: {'email': email, 'password': password});
        await _signedIn(r.data!);
      });

  Future<void> register({required String email, required String password, String name = '', bool marketing = false, String? locale}) =>
      _call(() => _dio.post<void>('/v1/auth/register', data: {
            'email': email,
            'password': password,
            'name': name,
            'accept_terms': true,
            'marketing_consent': marketing,
            'locale': ?locale,
          }));

  Future<void> verifyEmail(String email, String code) => _call(() async {
        final r = await _dio.post<Map<String, dynamic>>('/v1/auth/verify-email', data: {'email': email, 'code': code});
        await _signedIn(r.data!);
      });

  Future<void> forgotPassword(String email) => _call(() => _dio.post<void>('/v1/auth/password/forgot', data: {'email': email}));

  Future<void> resetPassword(String email, String code, String newPassword) => _call(() async {
        await _dio.post<void>('/v1/auth/password/reset', data: {'email': email, 'code': code, 'new_password': newPassword});
        await signInWithPassword(email, newPassword);
      });

  Future<void> signOut({bool everywhere = false}) async {
    try {
      await _dio.request<void>(everywhere ? '/v1/me/sessions' : '/v1/auth/logout',
          options: Options(method: everywhere ? 'DELETE' : 'POST'));
    } catch (_) {/* sign out locally regardless */}
    await _tokens.clear();
    await LocalCache.clearUserData();
    state = const AuthSignedOut();
  }

  /// Called after account deletion.
  Future<void> forgetLocally() async {
    await _tokens.clear();
    await LocalCache.clearUserData();
    state = const AuthSignedOut();
  }
}

final authControllerProvider = NotifierProvider<AuthController, AuthState>(AuthController.new);
