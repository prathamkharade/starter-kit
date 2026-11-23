import 'package:flutter/foundation.dart';
import 'package:flutter_dotenv/flutter_dotenv.dart';

/// Public runtime config. Source order: --dart-define > `env/<ENV>.env` > default.
class Env {
  Env._();

  static const flavor = String.fromEnvironment('ENV', defaultValue: 'dev');
  static const appVersion = String.fromEnvironment('APP_VERSION', defaultValue: '1.0.0');

  static Future<void> load() => dotenv.load(fileName: 'env/$flavor.env');

  static String _get(String key, String fallback) {
    final defined = _defines[key];
    if (defined != null && defined.isNotEmpty) return defined;
    final v = dotenv.env[key];
    return (v == null || v.isEmpty) ? fallback : v;
  }

  // String.fromEnvironment must be const per key, so list overridable keys here.
  static const _defines = <String, String>{
    'API_BASE_URL': String.fromEnvironment('API_BASE_URL'),
    'APP_NAME': String.fromEnvironment('APP_NAME'),
  };

  static String get appName => _get('APP_NAME', 'App');

  /// Web prod: '' = same origin. Mobile prod: API_BASE_URL_MOBILE.
  static String get apiBaseUrl {
    final base = _get('API_BASE_URL', '');
    if (!kIsWeb && base.isEmpty) return _get('API_BASE_URL_MOBILE', 'http://localhost:8080');
    return base;
  }

  static String get privacyUrl => _get('PRIVACY_URL', '');
  static String get termsUrl => _get('TERMS_URL', '');
  static String get supportEmail => _get('SUPPORT_EMAIL', '');
}
