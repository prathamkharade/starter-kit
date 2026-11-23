import 'package:dio/dio.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/network/api_client.dart';
import '../../core/network/api_exception.dart';
import '../../core/pagination/paged.dart';
import '../../shared/widgets/paged_list_view.dart';

class MediaItem {
  const MediaItem(this.json);
  final Map<String, dynamic> json;
  String get id => json['id'] as String;
  String get name => json['original_name'] as String;
  String get kind => json['kind'] as String;
  String get status => json['status'] as String;
  int get size => json['size_bytes'] as int? ?? 0;
  int get stored => json['stored_bytes'] as int? ?? 0;
  String? get url => json['url'] as String?;
  String? get thumb => (json['variants'] as Map?)?['thumb'] as String? ?? (json['variants'] as Map?)?['poster'] as String?;
}

class MediaController extends PagedNotifier<MediaItem> {
  @override
  Future<PageData<MediaItem>> fetch(ListQuery query, String? cursor) async {
    final r = await ref.read(dioProvider).get<Map<String, dynamic>>('/v1/media', queryParameters: query.toParams(cursor));
    return PageData.fromJson(r.data!, MediaItem.new);
  }

  /// Presigned direct upload: bytes go device → object storage, never through the API server.
  Future<void> upload(PlatformFile f, {void Function(double)? onProgress}) async {
    final dio = ref.read(dioProvider);
    final type = _mime(f.extension);
    final init = await dio.post<Map<String, dynamic>>('/v1/media/uploads',
        data: {'filename': f.name, 'content_type': type, 'size': f.size});
    final body = init.data!;
    final data = f.bytes ?? f.readStream;
    await Dio().put<void>( // plain Dio: no Authorization header (would break the S3 signature)
      body['upload_url'] as String,
      data: data,
      options: Options(headers: {'Content-Type': type, Headers.contentLengthHeader: f.size}),
      onSendProgress: (s, t) => onProgress?.call(t > 0 ? s / t : 0),
    );
    await dio.post<void>('/v1/media/${body['id']}/complete');
    await refresh();
  }

  Future<void> delete(String id) async {
    removeLocal((m) => m.id == id);
    await ref.read(dioProvider).delete<void>('/v1/media/$id');
  }

  static String _mime(String? ext) => switch (ext?.toLowerCase()) {
        'jpg' || 'jpeg' => 'image/jpeg',
        'png' => 'image/png',
        'gif' => 'image/gif',
        'webp' => 'image/webp',
        'heic' => 'image/heic',
        'mp4' => 'video/mp4',
        'mov' => 'video/quicktime',
        'webm' => 'video/webm',
        'mp3' => 'audio/mpeg',
        'm4a' => 'audio/mp4',
        'wav' => 'audio/wav',
        'pdf' => 'application/pdf',
        'zip' => 'application/zip',
        'txt' => 'text/plain',
        'csv' => 'text/csv',
        'json' => 'application/json',
        _ => 'application/octet-stream',
      };
}

final mediaProvider = NotifierProvider<MediaController, PagedState<MediaItem>>(MediaController.new);

class MediaPage extends ConsumerStatefulWidget {
  const MediaPage({super.key});
  @override
  ConsumerState<MediaPage> createState() => _MediaPageState();
}

class _MediaPageState extends ConsumerState<MediaPage> {
  double? _progress;

  Future<void> _pick() async {
    final res = await FilePicker.platform.pickFiles(withData: false, withReadStream: true);
    final f = res?.files.single;
    if (f == null) return;
    setState(() => _progress = 0);
    try {
      await ref.read(mediaProvider.notifier).upload(f, onProgress: (p) => setState(() => _progress = p));
    } catch (e) {
      if (mounted) showError(context, ApiException.from(e).message);
    } finally {
      if (mounted) setState(() => _progress = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(mediaProvider);
    final ctrl = ref.read(mediaProvider.notifier);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Files'),
        bottom: _progress == null ? null : PreferredSize(preferredSize: const Size.fromHeight(4), child: LinearProgressIndicator(value: _progress)),
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _progress == null ? _pick : null,
        icon: const Icon(Icons.upload_file),
        label: const Text('Upload'),
      ),
      body: ContentWidth(
        child: PagedListView<MediaItem>(
          state: state,
          onRefresh: ctrl.refresh,
          onLoadMore: ctrl.loadMore,
          empty: const EmptyState(
            icon: Icons.perm_media_outlined,
            title: 'No files yet',
            message: 'Photos, videos, PDFs, archives — anything. We compress automatically.',
          ),
          itemBuilder: (context, m) => ListTile(
            leading: SizedBox.square(
              dimension: 48,
              child: m.thumb != null
                  ? ClipRRect(
                      borderRadius: BorderRadius.circular(8),
                      child: Image.network(m.thumb!, fit: BoxFit.cover, semanticLabel: m.name, errorBuilder: (_, _, _) => const Icon(Icons.broken_image)),
                    )
                  : Icon(_icon(m.kind)),
            ),
            title: Text(m.name, maxLines: 1, overflow: TextOverflow.ellipsis),
            subtitle: Text(m.status == 'ready' ? '${_fmt(m.size)} → ${_fmt(m.stored)} stored' : m.status),
            onTap: m.url == null ? null : () => launchUrl(Uri.parse(m.url!)),
            trailing: IconButton(tooltip: 'Delete', icon: const Icon(Icons.delete_outline), onPressed: () => ctrl.delete(m.id)),
          ),
        ),
      ),
    );
  }

  static IconData _icon(String kind) => switch (kind) {
        'image' => Icons.image_outlined,
        'video' => Icons.movie_outlined,
        'audio' => Icons.audiotrack_outlined,
        'archive' => Icons.folder_zip_outlined,
        _ => Icons.description_outlined,
      };

  static String _fmt(int b) =>
      b < 1024 ? '$b B' : b < 1 << 20 ? '${(b / 1024).toStringAsFixed(0)} KB' : '${(b / (1 << 20)).toStringAsFixed(1)} MB';
}
