import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../../../shared/widgets/paged_list_view.dart';
import '../application/notes_controller.dart';
import '../data/notes_repository.dart';

class NotesPage extends ConsumerWidget {
  const NotesPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(notesProvider);
    final ctrl = ref.read(notesProvider.notifier);
    final archived = state.query.filters['archived'] == 'true';

    return Scaffold(
      appBar: AppBar(
        title: const Text('Notes'),
        actions: [
          PopupMenuButton<String>(
            tooltip: 'Sort',
            icon: const Icon(Icons.sort),
            initialValue: state.query.sort,
            onSelected: (s) => ctrl.setQuery(state.query.copyWith(sort: s)),
            itemBuilder: (_) => const [
              PopupMenuItem(value: '-updated_at', child: Text('Recently updated')),
              PopupMenuItem(value: '-created_at', child: Text('Newest first')),
              PopupMenuItem(value: 'title', child: Text('Title A–Z')),
            ],
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.go('/notes/new'),
        icon: const Icon(Icons.add),
        label: const Text('New note'),
      ),
      body: ContentWidth(
        child: PagedListView<Note>(
          state: state,
          onRefresh: ctrl.refresh,
          onLoadMore: ctrl.loadMore,
          header: SearchFilterBar(
            hint: 'Search notes',
            onSearch: (q) => ctrl.setQuery(state.query.copyWith(q: q, sort: q.isEmpty ? '-updated_at' : 'relevance')),
            filters: [
              FilterChip(
                label: const Text('Archived'),
                selected: archived,
                onSelected: (v) => ctrl.setQuery(state.query.copyWith(filters: {...state.query.filters, 'archived': '$v'})),
              ),
            ],
          ),
          empty: EmptyState(
            icon: Icons.sticky_note_2_outlined,
            title: state.query.q.isEmpty ? 'No notes yet' : 'No results',
            message: state.query.q.isEmpty ? 'Create your first note.' : 'Try a different search.',
          ),
          itemBuilder: (context, n) => Dismissible(
            key: ValueKey(n.id),
            direction: DismissDirection.endToStart,
            background: Container(
              alignment: Alignment.centerRight,
              padding: const EdgeInsets.only(right: 24),
              color: Theme.of(context).colorScheme.errorContainer,
              child: const Icon(Icons.delete_outline),
            ),
            onDismissed: (_) => ctrl.remove(n.id),
            child: ListTile(
              title: Text(n.title, maxLines: 1, overflow: TextOverflow.ellipsis),
              subtitle: Text(n.body, maxLines: 2, overflow: TextOverflow.ellipsis),
              trailing: n.pending ? const Tooltip(message: 'Waiting to sync', child: Icon(Icons.cloud_upload_outlined)) : null,
              onTap: n.pending ? null : () => context.go('/notes/${n.id}'),
            ),
          ),
        ),
      ),
    );
  }
}
