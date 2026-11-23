import 'dart:convert';

import 'package:hive_ce_flutter/hive_flutter.dart';

/// Tiny JSON key-value cache on Hive (IndexedDB on web, files on mobile).
/// Used for offline reads ("last known data") and small preferences.
/// Do not store secrets here — use SecureStore.
class LocalCache {
  LocalCache._();

  static late Box<String> _cache;
  static late Box<String> _prefs;

  static Future<void> init() async {
    await Hive.initFlutter();
    _cache = await Hive.openBox<String>('cache');
    _prefs = await Hive.openBox<String>('prefs');
  }

  static Future<void> putJson(String key, Object? value) =>
      _cache.put(key, jsonEncode({'v': value, 't': DateTime.now().toIso8601String()}));

  static T? getJson<T>(String key) {
    final raw = _cache.get(key);
    if (raw == null) return null;
    return (jsonDecode(raw) as Map<String, dynamic>)['v'] as T?;
  }

  static String? pref(String key) => _prefs.get(key);
  static Future<void> setPref(String key, String value) => _prefs.put(key, value);

  /// Wipe user data on sign-out / account deletion (GDPR: no leftovers on shared devices).
  static Future<void> clearUserData() async {
    await _cache.clear();
    if (Hive.isBoxOpen('outbox')) await Hive.box<String>('outbox').clear();
  }
}
