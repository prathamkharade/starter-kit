# Frontend (Flutter: web + Android + iOS)

Stack: flutter_riverpod 3 (Notifier/AsyncNotifier), go_router, dio, flutter_form_builder +
form_builder_validators, flutter_dotenv (public config only), permission_handler, hive_ce (offline),
flutter_secure_storage, connectivity_plus, file_picker.

## Run
`task web` (Chrome, port 5000 — matches `CORS_ORIGINS`) · `task mobile` (Android emulator → API at 10.0.2.2).
Physical phone: `flutter run --dart-define=API_BASE_URL=http://<your-PC-LAN-IP>:8080`.

## Customise the look (UI/UX)
- Brand colour, font, component shapes: `lib/app/theme/app_theme.dart` (one file, Material 3, light+dark).
- Spacing/radius/breakpoints: `Tokens` in the same file. Never hard-code colours/sizes in widgets.
- Navigation tabs: `destinations` in `lib/app/router.dart`. `AppShell` turns them into a bottom bar (phone),
  rail (tablet) or extended rail (desktop) automatically.
- Splash colour/text for web: `web/index.html` (pure CSS, shows instantly).
- Shared UX building blocks: `PagedListView`, `SearchFilterBar`, `EmptyState`, `ErrorState`, `ContentWidth`,
  `showError` (`lib/shared/widgets/`).
UX defaults baked in: one-field sign-in with code autofill, auto-submit on 6 digits, password optional,
48dp touch targets, readable max width, pull-to-refresh, infinite scroll, offline banner, dark mode,
system text scaling respected, i18n (en/de examples).

## Add a feature
`/add-entity` skill, or copy `lib/features/notes`: repository (dio + cache + outbox) → controller
(`extends PagedNotifier<T>`, implement `fetch`) → pages. Register routes in `app/router.dart` with
`deferred as` and wrap with `DeferredPage`.

## Web performance (fast first load)
1. **Instant paint**: CSS splash in `index.html`; no external fonts or scripts.
2. **Code splitting**: every non-first screen is a `deferred as` import → separate chunk downloaded on demand.
3. **Wasm build** (`--wasm`): Skwasm renderer in modern browsers, automatic JS fallback.
4. **Self-hosted engine** (`--no-web-resources-cdn` + `canvasKitBaseUrl` in `web/flutter_bootstrap.js`):
   no extra DNS/TLS to Google, and no IP leak to a third party (GDPR).
5. **Pre-compressed** brotli + gzip at build (`infra/web.Dockerfile`), served by Caddy; HTTP/3; Cloudflare CDN.
6. **Caching**: engine files immutable; app files `no-cache` + ETag (Flutter doesn't fingerprint file names).
7. **Service worker** (`web/sw.js`): network-first, offline fallback for the app shell.
8. Tree-shaken icons (default in release). Every package adds download weight: after adding one, compare
   the sizes of `build/web/main.dart.js` / `main.dart.wasm` and the deferred `*.part.js` chunks.
9. SEO/marketing pages: keep them as static HTML (any static site) at `/`, and mount the Flutter app at `/app`
   (`flutter build web --base-href /app/`) — Flutter web is not crawl-friendly.
Fonts: bundle your font in `assets/fonts` (pubspec) — never fetch Google Fonts at runtime.

## Offline
- **Reads**: repositories cache the first page per query and individual records (`LocalCache`, Hive). When the
  network fails, the cached copy is returned and lists show "Showing saved data".
- **Writes**: `Outbox.send()` tries immediately; if offline, persists `{method,path,body,id}` and replays in order
  on reconnect with `Idempotency-Key = id` (server dedupes). 409 conflicts / 4xx are surfaced via
  `outbox.failures` and dropped so the queue never blocks. Optimistic items show a "waiting to sync" icon.
- **Conflicts**: entities carry `version`; server rejects stale writes with 409 + `current`. Default policy:
  server wins, user is told and sees the latest copy. For collaborative editing consider CRDTs later.
- Web: data offline via IndexedDB; app shell offline via `sw.js`.
- Sign-out and account deletion wipe the cache and outbox (shared devices).

## Permissions
Use `PermissionService.ensure(context, Permission.x, rationale: '…')` right before the feature needs it.
Also declare them natively, or the request silently fails:
- Android `android/app/src/main/AndroidManifest.xml`: e.g. `<uses-permission android:name="android.permission.CAMERA"/>`,
  `POST_NOTIFICATIONS`, `ACCESS_FINE_LOCATION`. File picking needs none on Android 13+.
- iOS `ios/Runner/Info.plist`: `NSCameraUsageDescription`, `NSPhotoLibraryUsageDescription`,
  `NSLocationWhenInUseUsageDescription` (+ the permission_handler Podfile macros for permissions you use).
Ask only for what you use (GDPR data minimisation; store reviewers reject unused permissions).

## Accessibility (EU Accessibility Act → EN 301 549 / WCAG 2.1 AA, aim for 2.2)
Semantics labels on icon-only buttons (`tooltip:`), contrast from Material 3 schemes, focus order follows
layout, no info by colour alone, text scales to 200%, `prefers-reduced-motion` respected on web splash.
Test: Android TalkBack, iOS VoiceOver, keyboard-only on web, `flutter test` with `meetsGuideline`.

## i18n
Add strings to `lib/l10n/app_en.arb` (+ other languages), use `AppLocalizations.of(context).key`.
Generated code lands in `lib/l10n/gen` on `flutter pub get`/build.

## Security notes
`env/*.env` is shipped to users — public values only. Tokens: access in memory, refresh in Keychain/Keystore
(mobile) or HttpOnly cookie (web). Release builds: `--obfuscate --split-debug-info` (Taskfile `build:apk`).

## Imports
Use `package:material_ui/material_ui.dart` (and `package:cupertino_ui/cupertino_ui.dart` for Cupertino symbols), never `package:flutter/material.dart` — since Flutter 3.47 Material lives outside the SDK and mixing the two breaks types such as `InputDecoration` in `flutter_form_builder`.
