import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../../app/theme/app_theme.dart';
import '../../core/config/env.dart';
import '../../shared/widgets/paged_list_view.dart' show ContentWidth;
import '../auth/application/auth_controller.dart';

/// Landing screen after sign-in. Replace with your app's dashboard.
/// Kept in the main bundle (not deferred) because it is the first thing users see.
class HomePage extends ConsumerWidget {
  const HomePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authControllerProvider);
    final name = auth is AuthSignedIn ? (auth.user.name.isNotEmpty ? auth.user.name : auth.user.email ?? '') : '';
    final t = Theme.of(context).textTheme;
    return Scaffold(
      appBar: AppBar(title: Text(Env.appName)),
      body: ContentWidth(
        child: ListView(padding: const EdgeInsets.all(Tokens.md), children: [
          Text('Hi${name.isEmpty ? '' : ', $name'} 👋', style: t.headlineSmall),
          const SizedBox(height: Tokens.lg),
          _Tile(icon: Icons.sticky_note_2_outlined, title: 'Notes', subtitle: 'Search, filter, works offline', onTap: () => context.go('/notes')),
          _Tile(icon: Icons.perm_media_outlined, title: 'Files', subtitle: 'Upload anything, auto-compressed', onTap: () => context.go('/files')),
          _Tile(icon: Icons.privacy_tip_outlined, title: 'Privacy', subtitle: 'Consents, export, delete account', onTap: () => context.go('/settings')),
        ]),
      ),
    );
  }
}

class _Tile extends StatelessWidget {
  const _Tile({required this.icon, required this.title, required this.subtitle, required this.onTap});
  final IconData icon;
  final String title, subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Card(
        child: ListTile(leading: Icon(icon), title: Text(title), subtitle: Text(subtitle), trailing: const Icon(Icons.chevron_right), onTap: onTap),
      );
}
