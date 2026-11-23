import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:uuid/uuid.dart';

import '../../../core/network/api_client.dart';
import '../../../core/network/api_exception.dart';
import '../../../core/offline/outbox.dart';
import '../../../core/pagination/paged.dart';
import '../../../core/storage/local_cache.dart';

/// REFERENCE FEATURE (mirrors backend/internal/notes). Copy this folder for new entities.
class Note {
  const Note({
    required this.id,
    required this.title,
    this.body = '',
    this.tags = const [],
    this.archived = false,
    this.version = 0,
    this.updatedAt,
    this.pending = false,
  });

  final String id;
  final String title;
  final String body;
  final List<String> tags;
  final bool archived;
  final int version;
  final DateTime? updatedAt;
  final bool pending; // created/edited offline, not yet synced

  factory Note.fromJson(Map<String, dynamic> j) => Note(
        id: j['id'] as String,
        title: j['title'] as String,
        body: j['body'] as String? ?? '',
        tags: (j['tags'] as List?)?.cast<String>() ?? const [],
        archived: j['archived'] as bool? ?? false,
        version: j['version'] as int? ?? 0,
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? ''),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'title': title,
        'body': body,
        'tags': tags,
        'archived': archived,
        'version': version,
        'updated_at': updatedAt?.toIso8601String(),
      };

  Note copyWith({String? title, String? body, List<String>? tags, bool? archived, bool? pending}) => Note(
        id: id,
        title: title ?? this.title,
        body: body ?? this.body,
        tags: tags ?? this.tags,
        archived: archived ?? this.archived,
        version: version,
        updatedAt: DateTime.now(),
        pending: pending ?? this.pending,
      );
}

/// Network-first reads with an offline fallback (first page cached per query);
/// writes go through the Outbox so they survive being offline.
class NotesRepository {
  NotesRepository(this._dio, this._outbox);

  final Dio _dio;
  final Outbox _outbox;

  Future<PageData<Note>> list(ListQuery q, String? cursor) async {
    final cacheKey = 'notes?${q.cacheKey}';
    try {
      final r = await _dio.get<Map<String, dynamic>>('/v1/notes', queryParameters: q.toParams(cursor));
      if (cursor == null) await LocalCache.putJson(cacheKey, r.data);
      return PageData.fromJson(r.data!, Note.fromJson);
    } catch (e) {
      final cached = LocalCache.getJson<Map<String, dynamic>>(cacheKey);
      if (ApiException.from(e).isOffline && cursor == null && cached != null) {
        return PageData.fromJson(cached, Note.fromJson, fromCache: true);
      }
      rethrow;
    }
  }

  Future<Note> get(String id) async {
    try {
      final r = await _dio.get<Map<String, dynamic>>('/v1/notes/$id');
      await LocalCache.putJson('note:$id', r.data);
      return Note.fromJson(r.data!);
    } catch (e) {
      final cached = LocalCache.getJson<Map<String, dynamic>>('note:$id');
      if (cached != null) return Note.fromJson(cached);
      throw ApiException.from(e);
    }
  }

  /// Returns the saved note, or an optimistic local copy (pending=true) if offline.
  Future<Note> create(String title, String body, List<String> tags) async {
    final data = {'title': title, 'body': body, 'tags': tags};
    final res = await _outbox.send('POST', '/v1/notes', data);
    return res != null ? Note.fromJson(res) : Note(id: 'local-${const Uuid().v4()}', title: title, body: body, tags: tags, pending: true);
  }

  Future<Note> update(Note n) async {
    final res = await _outbox.send('PATCH', '/v1/notes/${n.id}',
        {'title': n.title, 'body': n.body, 'tags': n.tags, 'archived': n.archived, 'version': n.version});
    return res != null ? Note.fromJson(res) : n.copyWith(pending: true);
  }

  Future<void> delete(String id) => _outbox.send('DELETE', '/v1/notes/$id');
}

final notesRepositoryProvider = Provider((ref) => NotesRepository(ref.watch(dioProvider), ref.watch(outboxProvider)));
