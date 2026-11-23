import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../../app/router.dart';
import '../../app/theme/app_theme.dart';
import '../../core/offline/connectivity.dart';
import '../../core/offline/outbox.dart';

/// Adaptive navigation: bottom bar (phones) → rail (tablets) → extended rail
/// (desktop web). Same destinations everywhere = predictable navigation.
class AppShell extends ConsumerWidget {
  const AppShell({super.key, required this.shell});

  final StatefulNavigationShell shell;

  void _go(int i) => shell.goBranch(i, initialLocation: i == shell.currentIndex);

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.sizeOf(context).width;
    final online = ref.watch(onlineProvider).value ?? true;
    ref.watch(outboxProvider); // start replaying queued writes
    final body = Column(children: [
      if (!online) const OfflineBanner(),
      Expanded(child: shell),
    ]);

    if (width < Tokens.compact) {
      return Scaffold(
        body: body,
        bottomNavigationBar: NavigationBar(
          selectedIndex: shell.currentIndex,
          onDestinationSelected: _go,
          destinations: [
            for (final d in destinations)
              NavigationDestination(icon: Icon(d.icon), selectedIcon: Icon(d.selected), label: d.label),
          ],
        ),
      );
    }
    return Scaffold(
      body: Row(children: [
        NavigationRail(
          extended: width >= Tokens.expanded,
          selectedIndex: shell.currentIndex,
          onDestinationSelected: _go,
          labelType: width >= Tokens.expanded ? NavigationRailLabelType.none : NavigationRailLabelType.all,
          destinations: [
            for (final d in destinations)
              NavigationRailDestination(icon: Icon(d.icon), selectedIcon: Icon(d.selected), label: Text(d.label)),
          ],
        ),
        const VerticalDivider(width: 1),
        Expanded(child: body),
      ]),
    );
  }
}

class OfflineBanner extends ConsumerWidget {
  const OfflineBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final scheme = Theme.of(context).colorScheme;
    return Semantics(
      liveRegion: true,
      child: Material(
        color: scheme.tertiaryContainer,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: Tokens.md, vertical: Tokens.sm),
          child: ValueListenableBuilder<int>(
            valueListenable: ref.watch(outboxProvider).pending,
            builder: (_, n, _) => Row(children: [
              Icon(Icons.cloud_off, size: 18, color: scheme.onTertiaryContainer),
              const SizedBox(width: Tokens.sm),
              Expanded(
                child: Text(
                  n == 0 ? 'You are offline. Changes will sync when you reconnect.' : 'Offline — $n change(s) waiting to sync.',
                  style: TextStyle(color: scheme.onTertiaryContainer),
                ),
              ),
            ]),
          ),
        ),
      ),
    );
  }
}
