import 'package:cupertino_ui/cupertino_ui.dart' show CupertinoPageTransitionsBuilder;
import 'package:material_ui/material_ui.dart';

/// THE place to restyle the app. Change [seed] for brand colour, [fontFamily]
/// for typography (bundle the font in pubspec.yaml), tokens for spacing/shape.
/// Every widget reads from Theme/Tokens — never hard-code colours or sizes.
class AppTheme {
  AppTheme._();

  static const seed = Color(0xFF3B5BDB);
  static const String? fontFamily = null; // e.g. 'Inter' once bundled

  static ThemeData light() => _build(ColorScheme.fromSeed(seedColor: seed));
  static ThemeData dark() => _build(ColorScheme.fromSeed(seedColor: seed, brightness: Brightness.dark));

  static ThemeData _build(ColorScheme scheme) {
    final radius = BorderRadius.circular(Tokens.radius);
    return ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      fontFamily: fontFamily,
      visualDensity: VisualDensity.standard,
      materialTapTargetSize: MaterialTapTargetSize.padded, // ≥48dp targets (WCAG 2.5.8)
      scaffoldBackgroundColor: scheme.surface,
      appBarTheme: AppBarTheme(centerTitle: false, backgroundColor: scheme.surface, scrolledUnderElevation: 1),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: scheme.surfaceContainerHighest.withValues(alpha: .5),
        border: OutlineInputBorder(borderRadius: radius, borderSide: BorderSide.none),
        focusedBorder: OutlineInputBorder(borderRadius: radius, borderSide: BorderSide(color: scheme.primary, width: 2)),
        errorBorder: OutlineInputBorder(borderRadius: radius, borderSide: BorderSide(color: scheme.error)),
        contentPadding: const EdgeInsets.symmetric(horizontal: Tokens.md, vertical: Tokens.md),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(minimumSize: const Size(64, 52), shape: RoundedRectangleBorder(borderRadius: radius)),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(minimumSize: const Size(64, 52), shape: RoundedRectangleBorder(borderRadius: radius)),
      ),
      cardTheme: CardThemeData(elevation: 0, shape: RoundedRectangleBorder(borderRadius: radius), color: scheme.surfaceContainerLow),
      listTileTheme: ListTileThemeData(shape: RoundedRectangleBorder(borderRadius: radius)),
      snackBarTheme: const SnackBarThemeData(behavior: SnackBarBehavior.floating),
      pageTransitionsTheme: const PageTransitionsTheme(builders: {
        TargetPlatform.android: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
      }),
    );
  }
}

/// Design tokens: spacing scale (4pt grid), radius, breakpoints, durations.
class Tokens {
  Tokens._();
  static const double xs = 4, sm = 8, md = 16, lg = 24, xl = 32, xxl = 48;
  static const double radius = 14;
  static const double maxContentWidth = 720; // readable line length on desktop
  static const double compact = 600, expanded = 1200; // Material 3 window size classes
  static const fast = Duration(milliseconds: 150), normal = Duration(milliseconds: 250);
}
