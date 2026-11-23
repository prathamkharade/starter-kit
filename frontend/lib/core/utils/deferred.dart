import 'package:material_ui/material_ui.dart';

/// Loads a `deferred as` library before building its page. On web each
/// deferred library becomes a separate JS/Wasm chunk, so the first load only
/// downloads the code for the first screen ("code splitting").
///
///   import '../features/notes/notes_page.dart' deferred as notes;
///   GoRoute(path: '/notes', builder: (_, __) => DeferredPage(notes.loadLibrary, () => notes.NotesPage()))
class DeferredPage extends StatefulWidget {
  const DeferredPage(this.loader, this.builder, {super.key});

  final Future<void> Function() loader;
  final Widget Function() builder;

  @override
  State<DeferredPage> createState() => _DeferredPageState();
}

class _DeferredPageState extends State<DeferredPage> {
  late Future<void> _future = widget.loader();

  @override
  Widget build(BuildContext context) => FutureBuilder<void>(
        future: _future,
        builder: (context, snap) {
          if (snap.connectionState == ConnectionState.done && !snap.hasError) return widget.builder();
          if (snap.hasError) {
            return Center(
              child: TextButton(
                onPressed: () => setState(() => _future = widget.loader()),
                child: const Text('Could not load this page. Check your connection.'),
              ),
            );
          }
          return const Center(child: CircularProgressIndicator());
        },
      );
}
