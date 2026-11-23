import 'package:flutter_form_builder/flutter_form_builder.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:form_builder_validators/form_builder_validators.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../../../app/theme/app_theme.dart';
import '../../../core/network/api_exception.dart';
import '../../../shared/widgets/paged_list_view.dart';
import '../application/notes_controller.dart';
import '../data/notes_repository.dart';

final _noteProvider = FutureProvider.autoDispose.family<Note?, String>(
  (ref, id) => id == 'new' ? Future.value(null) : ref.read(notesRepositoryProvider).get(id),
);

class NoteEditPage extends ConsumerStatefulWidget {
  const NoteEditPage({super.key, required this.id});
  final String id;

  @override
  ConsumerState<NoteEditPage> createState() => _NoteEditPageState();
}

class _NoteEditPageState extends ConsumerState<NoteEditPage> {
  final _form = GlobalKey<FormBuilderState>();
  bool _busy = false;

  Future<void> _save(Note? existing) async {
    if (!(_form.currentState?.saveAndValidate() ?? false)) return;
    final v = _form.currentState!.value;
    final tags = ((v['tags'] as String?) ?? '').split(',').map((t) => t.trim()).where((t) => t.isNotEmpty).toList();
    setState(() => _busy = true);
    try {
      final saved = await ref
          .read(notesProvider.notifier)
          .save(existing: existing, title: v['title'] as String, body: (v['body'] as String?) ?? '', tags: tags);
      if (!mounted) return;
      if (saved.pending) showError(context, 'Saved offline — will sync automatically.');
      context.go('/notes');
    } on ApiException catch (e) {
      if (!mounted) return;
      if (e.isConflict) {
        showError(context, 'This note was changed on another device. Reloaded the latest version.');
        ref.invalidate(_noteProvider(widget.id));
      } else {
        e.fields.forEach((k, m) => _form.currentState?.fields[k]?.invalidate(m));
        showError(context, e.message);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final note = ref.watch(_noteProvider(widget.id));
    return Scaffold(
      appBar: AppBar(title: Text(widget.id == 'new' ? 'New note' : 'Edit note')),
      body: note.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => ErrorState(message: ApiException.from(e).message, onRetry: () => ref.invalidate(_noteProvider(widget.id))),
        data: (n) => SingleChildScrollView(
          padding: const EdgeInsets.all(Tokens.md),
          child: ContentWidth(
            child: FormBuilder(
              key: _form,
              initialValue: {'title': n?.title ?? '', 'body': n?.body ?? '', 'tags': n?.tags.join(', ') ?? ''},
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                FormBuilderTextField(
                  name: 'title',
                  autofocus: n == null,
                  decoration: const InputDecoration(labelText: 'Title'),
                  validator: FormBuilderValidators.compose([FormBuilderValidators.required(), FormBuilderValidators.maxLength(200)]),
                ),
                const SizedBox(height: Tokens.md),
                FormBuilderTextField(
                  name: 'body',
                  minLines: 6,
                  maxLines: null,
                  decoration: const InputDecoration(labelText: 'Note', alignLabelWithHint: true),
                ),
                const SizedBox(height: Tokens.md),
                FormBuilderTextField(
                  name: 'tags',
                  decoration: const InputDecoration(labelText: 'Tags', helperText: 'Comma separated'),
                ),
                const SizedBox(height: Tokens.lg),
                FilledButton(onPressed: _busy ? null : () => _save(n), child: const Text('Save')),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
