import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../core/storage/local_cache.dart';

/// Device-level UI preferences (not synced to the server).
class Preferences {
  const Preferences({this.themeMode = ThemeMode.system, this.locale});
  final ThemeMode themeMode;
  final Locale? locale; // null = follow the device language
}

class PreferencesController extends Notifier<Preferences> {
  @override
  Preferences build() {
    final mode = ThemeMode.values.asNameMap()[LocalCache.pref('themeMode')] ?? ThemeMode.system;
    final lang = LocalCache.pref('locale');
    return Preferences(themeMode: mode, locale: (lang == null || lang.isEmpty) ? null : Locale(lang));
  }

  Future<void> setThemeMode(ThemeMode m) async {
    state = Preferences(themeMode: m, locale: state.locale);
    await LocalCache.setPref('themeMode', m.name);
  }

  Future<void> setLocale(Locale? l) async {
    state = Preferences(themeMode: state.themeMode, locale: l);
    await LocalCache.setPref('locale', l?.languageCode ?? '');
  }
}

final preferencesProvider = NotifierProvider<PreferencesController, Preferences>(PreferencesController.new);
