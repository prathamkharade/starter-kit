import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_web_plugins/url_strategy.dart';
import 'package:material_ui/material_ui.dart';

import 'core/config/env.dart';
import 'core/storage/local_cache.dart';

/// Everything that must happen before the first frame. Keep it minimal:
/// every millisecond here delays first paint (especially on web).
Future<void> bootstrap(Widget Function() app) async {
  WidgetsFlutterBinding.ensureInitialized();
  usePathUrlStrategy(); // /notes instead of /#/notes (server rewrites to index.html)

  await Future.wait([Env.load(), LocalCache.init()]);

  FlutterError.onError = (details) {
    FlutterError.presentError(details);
    // Hook crash reporting here — only if the user consented to "crash_reports".
  };
  PlatformDispatcher.instance.onError = (error, stack) {
    debugPrint('Uncaught: $error');
    return true;
  };

  runApp(ProviderScope(child: app()));
}
