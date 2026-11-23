import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../core/utils/deferred.dart';
import '../features/auth/application/auth_controller.dart';
// Heavier / less-visited screens are deferred: separate download chunks on web.
import '../features/auth/presentation/register_page.dart' deferred as register;
import '../features/auth/presentation/sign_in_page.dart';
import '../features/auth/presentation/verify_code_page.dart';
import '../features/home/home_page.dart';
import '../features/media/media_page.dart' deferred as media;
import '../features/notes/presentation/note_edit_page.dart' deferred as note_edit;
import '../features/notes/presentation/notes_page.dart' deferred as notes;
import '../features/settings/settings_page.dart' deferred as settings;
import '../shared/widgets/app_shell.dart';

/// Bottom-nav / rail destinations. Add a tab = add an entry here + a branch below.
const destinations = [
  (icon: Icons.home_outlined, selected: Icons.home, label: 'Home'),
  (icon: Icons.sticky_note_2_outlined, selected: Icons.sticky_note_2, label: 'Notes'),
  (icon: Icons.perm_media_outlined, selected: Icons.perm_media, label: 'Files'),
  (icon: Icons.settings_outlined, selected: Icons.settings, label: 'Settings'),
];

const _publicPaths = {'/sign-in', '/verify', '/register'};

final routerProvider = Provider<GoRouter>((ref) {
  // Bridge Riverpod → Listenable so GoRouter re-runs redirect on auth changes.
  final refresh = ValueNotifier<int>(0);
  ref.listen(authControllerProvider, (_, _) => refresh.value++);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    initialLocation: '/',
    refreshListenable: refresh,
    redirect: (context, state) {
      final auth = ref.read(authControllerProvider);
      final path = state.matchedLocation;
      final isPublic = _publicPaths.contains(path);
      if (auth is AuthUnknown) return path == '/splash' ? null : '/splash?from=${Uri.encodeComponent(state.uri.toString())}';
      if (auth is AuthSignedOut && !isPublic) {
        final from = path == '/splash' ? state.uri.queryParameters['from'] : state.uri.toString();
        return '/sign-in${from == null || from == '/' ? '' : '?from=${Uri.encodeComponent(from)}'}';
      }
      if (auth is AuthSignedIn && (isPublic || path == '/splash')) {
        return state.uri.queryParameters['from'] ?? '/';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/splash', builder: (_, _) => const Scaffold(body: Center(child: CircularProgressIndicator()))),
      GoRoute(path: '/sign-in', builder: (_, _) => const SignInPage()),
      GoRoute(path: '/verify', builder: (_, s) => VerifyCodePage(args: s.extra as VerifyArgs? ?? VerifyArgs.empty)),
      GoRoute(path: '/register', builder: (_, _) => DeferredPage(register.loadLibrary, () => register.RegisterPage())),
      StatefulShellRoute.indexedStack(
        builder: (_, _, shell) => AppShell(shell: shell),
        branches: [
          StatefulShellBranch(routes: [GoRoute(path: '/', builder: (_, _) => const HomePage())]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: '/notes',
              builder: (_, _) => DeferredPage(notes.loadLibrary, () => notes.NotesPage()),
              routes: [
                GoRoute(
                  path: ':id',
                  builder: (_, s) => DeferredPage(note_edit.loadLibrary, () => note_edit.NoteEditPage(id: s.pathParameters['id']!)),
                ),
              ],
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(path: '/files', builder: (_, _) => DeferredPage(media.loadLibrary, () => media.MediaPage())),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(path: '/settings', builder: (_, _) => DeferredPage(settings.loadLibrary, () => settings.SettingsPage())),
          ]),
        ],
      ),
    ],
    errorBuilder: (_, s) => Scaffold(body: Center(child: Text('Page not found: ${s.uri.path}'))),
  );
});
