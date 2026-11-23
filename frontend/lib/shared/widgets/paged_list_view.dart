import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../../app/theme/app_theme.dart';
import '../../core/pagination/paged.dart';

/// Generic list with pull-to-refresh, infinite scroll, empty/error/offline
/// states. Pair with a [PagedNotifier]:
///
///   final s = ref.watch(notesProvider);
///   PagedListView(state: s, onRefresh: n.refresh, onLoadMore: n.loadMore,
///                 itemBuilder: (c, note) => NoteTile(note));
class PagedListView<T> extends StatefulWidget {
  const PagedListView({
    super.key,
    required this.state,
    required this.itemBuilder,
    required this.onRefresh,
    required this.onLoadMore,
    this.empty = const EmptyState(icon: Icons.inbox_outlined, title: 'Nothing here yet'),
    this.header,
  });

  final PagedState<T> state;
  final Widget Function(BuildContext, T) itemBuilder;
  final Future<void> Function() onRefresh;
  final Future<void> Function() onLoadMore;
  final Widget empty;
  final Widget? header;

  @override
  State<PagedListView<T>> createState() => _PagedListViewState<T>();
}

class _PagedListViewState<T> extends State<PagedListView<T>> {
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    _scroll.addListener(() {
      if (_scroll.position.extentAfter < 600) unawaited(widget.onLoadMore());
    });
  }

  @override
  void dispose() {
    _scroll.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.state;
    final Widget content;
    if (s.loading && s.items.isEmpty) {
      content = const SliverFillRemaining(child: Center(child: CircularProgressIndicator()));
    } else if (s.error != null && s.items.isEmpty) {
      content = SliverFillRemaining(
        child: ErrorState(message: s.error!.message, onRetry: widget.onRefresh),
      );
    } else if (s.items.isEmpty) {
      content = SliverFillRemaining(hasScrollBody: false, child: widget.empty);
    } else {
      content = SliverList.builder(
        itemCount: s.items.length + 1,
        itemBuilder: (c, i) {
          if (i < s.items.length) return widget.itemBuilder(c, s.items[i]);
          if (s.loadingMore) {
            return const Padding(padding: EdgeInsets.all(Tokens.md), child: Center(child: CircularProgressIndicator()));
          }
          if (s.error != null) {
            return TextButton(onPressed: widget.onLoadMore, child: const Text('Could not load more — retry'));
          }
          return const SizedBox(height: Tokens.xxl);
        },
      );
    }
    return RefreshIndicator(
      onRefresh: widget.onRefresh,
      child: CustomScrollView(
        controller: _scroll,
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          if (widget.header != null) SliverToBoxAdapter(child: widget.header),
          if (s.stale)
            const SliverToBoxAdapter(
              child: ListTile(dense: true, leading: Icon(Icons.history), title: Text('Showing saved data (offline)')),
            ),
          content,
        ],
      ),
    );
  }
}

class EmptyState extends StatelessWidget {
  const EmptyState({super.key, required this.icon, required this.title, this.message, this.action});
  final IconData icon;
  final String title;
  final String? message;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(Tokens.xl),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Icon(icon, size: 56, color: Theme.of(context).colorScheme.outline),
          const SizedBox(height: Tokens.md),
          Text(title, style: t.titleMedium, textAlign: TextAlign.center),
          if (message != null) ...[const SizedBox(height: Tokens.sm), Text(message!, textAlign: TextAlign.center)],
          if (action != null) ...[const SizedBox(height: Tokens.lg), action!],
        ]),
      ),
    );
  }
}

class ErrorState extends StatelessWidget {
  const ErrorState({super.key, required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => EmptyState(
        icon: Icons.error_outline,
        title: 'Something went wrong',
        message: message,
        action: OutlinedButton(onPressed: onRetry, child: const Text('Try again')),
      );
}

/// Search field with debounce + optional filter chips row.
class SearchFilterBar extends StatefulWidget {
  const SearchFilterBar({super.key, required this.onSearch, this.hint = 'Search', this.filters = const []});
  final ValueChanged<String> onSearch;
  final String hint;
  final List<Widget> filters; // e.g. FilterChip widgets

  @override
  State<SearchFilterBar> createState() => _SearchFilterBarState();
}

class _SearchFilterBarState extends State<SearchFilterBar> {
  Timer? _debounce;
  final _c = TextEditingController();

  @override
  void dispose() {
    _debounce?.cancel();
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(Tokens.md, Tokens.sm, Tokens.md, 0),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        TextField(
          controller: _c,
          textInputAction: TextInputAction.search,
          decoration: InputDecoration(
            hintText: widget.hint,
            prefixIcon: const Icon(Icons.search),
            suffixIcon: ValueListenableBuilder(
              valueListenable: _c,
              builder: (_, v, _) => v.text.isEmpty
                  ? const SizedBox.shrink()
                  : IconButton(
                      tooltip: 'Clear',
                      icon: const Icon(Icons.close),
                      onPressed: () {
                        _c.clear();
                        widget.onSearch('');
                      },
                    ),
            ),
          ),
          onChanged: (v) {
            _debounce?.cancel();
            _debounce = Timer(const Duration(milliseconds: 350), () => widget.onSearch(v));
          },
          onSubmitted: widget.onSearch,
        ),
        if (widget.filters.isNotEmpty)
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(vertical: Tokens.sm),
            child: Row(children: [for (final f in widget.filters) Padding(padding: const EdgeInsets.only(right: Tokens.sm), child: f)]),
          ),
      ]),
    );
  }
}

/// Constrains content to a readable width on large screens.
class ContentWidth extends StatelessWidget {
  const ContentWidth({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => Center(
        child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: Tokens.maxContentWidth), child: child),
      );
}

void showError(BuildContext context, Object message) {
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text('$message')));
}
