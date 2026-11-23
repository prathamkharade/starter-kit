import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../config/env.dart';
import '../storage/secure_store.dart';

/// Holds the session tokens and performs single-flight refreshes.
/// Access token: memory only. Refresh token: HttpOnly cookie (web) or secure storage (mobile).
class TokenManager {
  TokenManager(this._store);

  final SecureStore _store;
  String? _access;
  Future<bool>? _refreshing;
  final _expired = StreamController<void>.broadcast();

  /// Emits when the session can no longer be refreshed (→ signed out).
  Stream<void> get onSessionExpired => _expired.stream;
  String? get accessToken => _access;

  late final Dio _bare = Dio(_baseOptions());

  Future<void> save(Map<String, dynamic> tokenResponse) async {
    _access = tokenResponse['access_token'] as String?;
    final rt = tokenResponse['refresh_token'] as String?;
    if (rt != null) await _store.writeRefresh(rt);
  }

  Future<void> clear() async {
    _access = null;
    await _store.writeRefresh(null);
  }

  /// Returns the refreshed token response, or null if the session is gone.
  Future<Map<String, dynamic>?> refreshSession() async {
    Map<String, dynamic>? result;
    Future<bool> run() async {
      for (var attempt = 0; attempt < 2; attempt++) {
        try {
          final rt = await _store.readRefresh();
          if (!kIsWeb && rt == null) return false;
          final res = await _bare.post<Map<String, dynamic>>('/v1/auth/refresh',
              data: kIsWeb ? null : {'refresh_token': rt});
          result = res.data;
          await save(result!);
          return true;
        } on DioException catch (e) {
          if (e.response?.statusCode == 409) {
            await Future<void>.delayed(const Duration(milliseconds: 400)); // another tab won the race
            continue;
          }
          if (e.response == null) rethrow; // offline: keep tokens, try later
          return false;
        }
      }
      return false;
    }

    _refreshing ??= run().whenComplete(() => _refreshing = null);
    final ok = await _refreshing!;
    if (!ok) {
      await clear();
      _expired.add(null);
      return null;
    }
    return result ?? {'access_token': _access};
  }
}

BaseOptions _baseOptions() => BaseOptions(
      baseUrl: Env.apiBaseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      contentType: Headers.jsonContentType,
      headers: {if (!kIsWeb) 'X-Client': 'mobile'},
      extra: {'withCredentials': true}, // web: send the refresh cookie (dio browser adapter)
    );

final secureStoreProvider = Provider((ref) => SecureStore());
final tokenManagerProvider = Provider((ref) => TokenManager(ref.watch(secureStoreProvider)));

/// The app-wide Dio. Adds the bearer token and transparently refreshes on 401.
final dioProvider = Provider<Dio>((ref) {
  final tokens = ref.watch(tokenManagerProvider);
  final dio = Dio(_baseOptions());
  // Retries go through a plain Dio: re-entering a QueuedInterceptor from its own
  // onError can deadlock if the retry fails too.
  final retryDio = Dio(_baseOptions());
  dio.interceptors.add(QueuedInterceptorsWrapper(
    onRequest: (options, handler) {
      final t = tokens.accessToken;
      if (t != null) options.headers['Authorization'] = 'Bearer $t';
      handler.next(options);
    },
    onError: (err, handler) async {
      final req = err.requestOptions;
      final unauthorized = err.response?.statusCode == 401;
      if (!unauthorized || req.extra['retried'] == true || req.path.contains('/auth/')) {
        return handler.next(err);
      }
      try {
        final refreshed = await tokens.refreshSession();
        if (refreshed == null) return handler.next(err);
        req.extra['retried'] = true;
        req.headers['Authorization'] = 'Bearer ${tokens.accessToken}';
        handler.resolve(await retryDio.fetch<dynamic>(req));
      } catch (_) {
        handler.next(err);
      }
    },
  ));
  if (kDebugMode) {
    dio.interceptors.add(LogInterceptor(requestBody: false, responseBody: false, requestHeader: false));
  }
  return dio;
});
