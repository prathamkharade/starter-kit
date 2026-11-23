import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../app/preferences.dart';
import '../../app/theme/app_theme.dart';
import '../../core/config/env.dart';
import '../../core/network/api_client.dart';
import '../../core/network/api_exception.dart';
import '../../core/utils/save_file.dart';
import '../../l10n/gen/app_localizations.dart';
import '../../shared/widgets/paged_list_view.dart';
import '../auth/application/auth_controller.dart';

/// Consent purposes shown to users. Keys must match backend togglePurposes (internal/users).
const _purposes = {
  'marketing': ('Product news by email', 'Occasional updates about new features.'),
  'analytics': ('Usage analytics', 'Anonymous statistics that help us improve the app.'),
  'crash_reports': ('Crash reports', 'Send diagnostics when the app crashes.'),
};

final _consentsProvider = FutureProvider.autoDispose<Map<String, bool>>((ref) async {
  final r = await ref.read(dioProvider).get<Map<String, dynamic>>('/v1/me/consents');
  return (r.data!['consents'] as Map).map((k, v) => MapEntry(k as String, v as bool));
});

class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final prefs = ref.watch(preferencesProvider);
    final consents = ref.watch(_consentsProvider);
    final auth = ref.watch(authControllerProvider);
    final user = auth is AuthSignedIn ? auth.user : null;

    return Scaffold(
      appBar: AppBar(title: Text(l.settings)),
      body: ContentWidth(
        child: ListView(padding: const EdgeInsets.symmetric(vertical: Tokens.sm), children: [
          if (user != null) ListTile(leading: const CircleAvatar(child: Icon(Icons.person)), title: Text(user.name.isEmpty ? (user.email ?? '') : user.name), subtitle: user.name.isEmpty ? null : Text(user.email ?? '')),
          _Header(l.appearance),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: Tokens.md),
            child: SegmentedButton<ThemeMode>(
              segments: const [
                ButtonSegment(value: ThemeMode.system, icon: Icon(Icons.brightness_auto), label: Text('Auto')),
                ButtonSegment(value: ThemeMode.light, icon: Icon(Icons.light_mode), label: Text('Light')),
                ButtonSegment(value: ThemeMode.dark, icon: Icon(Icons.dark_mode), label: Text('Dark')),
              ],
              selected: {prefs.themeMode},
              onSelectionChanged: (s) => ref.read(preferencesProvider.notifier).setThemeMode(s.first),
            ),
          ),
          ListTile(
            leading: const Icon(Icons.language),
            title: Text(l.language),
            trailing: DropdownButton<String>(
              value: prefs.locale?.languageCode ?? '',
              underline: const SizedBox.shrink(),
              items: [
                DropdownMenuItem(value: '', child: Text(l.systemDefault)),
                for (final loc in AppLocalizations.supportedLocales)
                  DropdownMenuItem(value: loc.languageCode, child: Text(loc.languageCode.toUpperCase())),
              ],
              onChanged: (v) => ref.read(preferencesProvider.notifier).setLocale(v == null || v.isEmpty ? null : Locale(v)),
            ),
          ),
          _Header(l.privacy),
          consents.when(
            loading: () => const ListTile(title: LinearProgressIndicator()),
            error: (e, _) => ListTile(title: Text(ApiException.from(e).message), trailing: TextButton(onPressed: () => ref.invalidate(_consentsProvider), child: Text(l.retry))),
            data: (c) => Column(children: [
              for (final p in _purposes.entries)
                SwitchListTile(
                  title: Text(p.value.$1),
                  subtitle: Text(p.value.$2),
                  value: c[p.key] ?? false,
                  onChanged: (v) async {
                    await ref.read(dioProvider).put<void>('/v1/me/consents', data: {'purpose': p.key, 'granted': v});
                    ref.invalidate(_consentsProvider);
                  },
                ),
            ]),
          ),
          ListTile(leading: const Icon(Icons.download_outlined), title: Text(l.downloadData), onTap: () => _export(context, ref)),
          ListTile(leading: const Icon(Icons.policy_outlined), title: const Text('Privacy Policy'), onTap: () => launchUrl(Uri.parse(Env.privacyUrl))),
          ListTile(leading: const Icon(Icons.gavel_outlined), title: const Text('Terms'), onTap: () => launchUrl(Uri.parse(Env.termsUrl))),
          const Divider(),
          ListTile(leading: const Icon(Icons.logout), title: Text(l.signOut), onTap: () => ref.read(authControllerProvider.notifier).signOut()),
          ListTile(leading: const Icon(Icons.devices), title: Text(l.signOutEverywhere), onTap: () => ref.read(authControllerProvider.notifier).signOut(everywhere: true)),
          ListTile(
            leading: Icon(Icons.delete_forever_outlined, color: Theme.of(context).colorScheme.error),
            title: Text(l.deleteAccount, style: TextStyle(color: Theme.of(context).colorScheme.error)),
            onTap: () => _delete(context, ref),
          ),
          const SizedBox(height: Tokens.lg),
          Center(child: Text('${Env.appName} ${Env.appVersion}', style: Theme.of(context).textTheme.bodySmall)),
        ]),
      ),
    );
  }

  Future<void> _export(BuildContext context, WidgetRef ref) async {
    try {
      final r = await ref.read(dioProvider).get<List<int>>('/v1/me/export', options: Options(responseType: ResponseType.bytes));
      final where = await saveFile(Uint8List.fromList(r.data!), 'my-data.json', 'application/json');
      if (context.mounted) showError(context, 'Saved to $where');
    } catch (e) {
      if (context.mounted) showError(context, ApiException.from(e).message);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref) async {
    final l = AppLocalizations.of(context);
    final c = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(l.deleteAccount),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          Text(l.deleteAccountBody),
          const SizedBox(height: Tokens.md),
          TextField(controller: c, decoration: const InputDecoration(hintText: 'DELETE')),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          ValueListenableBuilder(
            valueListenable: c,
            builder: (_, v, _) => FilledButton(
              style: FilledButton.styleFrom(backgroundColor: Theme.of(ctx).colorScheme.error),
              onPressed: v.text == 'DELETE' ? () => Navigator.pop(ctx, true) : null,
              child: Text(l.deleteAccount),
            ),
          ),
        ],
      ),
    );
    c.dispose();
    if (ok != true) return;
    try {
      await ref.read(dioProvider).delete<void>('/v1/me', data: {'confirm': 'DELETE'});
      await ref.read(authControllerProvider.notifier).forgetLocally();
    } catch (e) {
      if (context.mounted) showError(context, ApiException.from(e).message);
    }
  }
}

class _Header extends StatelessWidget {
  const _Header(this.text);
  final String text;
  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.fromLTRB(Tokens.md, Tokens.lg, Tokens.md, Tokens.sm),
        child: Text(text, style: Theme.of(context).textTheme.titleSmall?.copyWith(color: Theme.of(context).colorScheme.primary)),
      );
}
