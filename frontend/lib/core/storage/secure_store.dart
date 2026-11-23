import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Refresh-token storage. Mobile: Keychain/Keystore. Web: nothing is stored —
/// the refresh token lives in an HttpOnly cookie JavaScript cannot read (XSS-safe).
class SecureStore {
  static const _key = 'refresh_token';
  final _storage = const FlutterSecureStorage();

  Future<String?> readRefresh() async => kIsWeb ? null : _storage.read(key: _key);

  Future<void> writeRefresh(String? token) async {
    if (kIsWeb) return;
    if (token == null) {
      await _storage.delete(key: _key);
    } else {
      await _storage.write(key: _key, value: token);
    }
  }
}
