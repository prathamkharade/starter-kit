import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/pagination/paged.dart';
import '../data/notes_repository.dart';

/// All list behaviour (search, filters, infinite scroll, offline cache) comes
/// from PagedNotifier — a new list screen is ~10 lines.
class NotesController extends PagedNotifier<Note> {
  @override
  ListQuery get initialQuery => const ListQuery(filters: {'archived': 'false'}, sort: '-updated_at');

  @override
  Future<PageData<Note>> fetch(ListQuery query, String? cursor) => ref.read(notesRepositoryProvider).list(query, cursor);

  Future<Note> save({Note? existing, required String title, required String body, required List<String> tags}) async {
    final repo = ref.read(notesRepositoryProvider);
    final saved = existing == null
        ? await repo.create(title, body, tags)
        : await repo.update(existing.copyWith(title: title, body: body, tags: tags));
    upsertLocal(saved, (a, b) => a.id == b.id);
    return saved;
  }

  Future<void> remove(String id) async {
    removeLocal((n) => n.id == id);
    await ref.read(notesRepositoryProvider).delete(id);
  }
}

final notesProvider = NotifierProvider<NotesController, PagedState<Note>>(NotesController.new);
