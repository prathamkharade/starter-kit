import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:hive_ce/hive.dart';
import 'package:uuid/uuid.dart';

import '../network/api_client.dart';
import '../network/api_exception.dart';
import 'connectivity.dart';

/// A queued write made while offline.
class PendingWrite {
  PendingWrite({required this.id, required this.method, required this.path, this.body, required this.createdAt});

  final String id; // also the Idempotency-Key → safe to replay
  final String method;
  final String path;
  final Map<String, dynamic>? body;
  final DateTime createdAt;

  Map<String, dynamic> toJson() =>
      {'id': id, 'method': method, 'path': path, 'body': body, 'createdAt': createdAt.toIso8601String()};

  factory PendingWrite.fromJson(Map<String, dynamic> j) => PendingWrite(
        id: j['id'] as String,
        method: j['method'] as String,
        path: j['path'] as String,
        body: (j['body'] as Map?)?.cast<String, dynamic>(),
        createdAt: DateTime.parse(j['createdAt'] as String),
      );
}

/// Offline write queue ("outbox pattern"). Writes are persisted, replayed in
/// order when the network returns, and deduplicated server-side by the
/// Idempotency-Key header. Conflicts (409) and validation errors are reported
/// via [failures] and dropped so one bad write can't block the queue.
class Outbox {
  Outbox(this._dio);

  final Dio _dio;
  Box<String>? _box;
  bool _flushing = false;
  final _failures = StreamController<(PendingWrite, ApiException)>.broadcast();
  final pending = ValueNotifier<int>(0);

  Stream<(PendingWrite, ApiException)> get failures => _failures.stream;

  Future<Box<String>> _open() async {
    final box = _box ??= await Hive.openBox<String>('outbox');
    pending.value = box.length;
    return box;
  }

  Future<PendingWrite> enqueue(String method, String path, [Map<String, dynamic>? body]) async {
    final w = PendingWrite(id: const Uuid().v4(), method: method, path: path, body: body, createdAt: DateTime.now());
    final box = await _open();
    await box.put('${w.createdAt.microsecondsSinceEpoch}-${w.id}', jsonEncode(w.toJson())); // keys sort by time
    pending.value = box.length;
    return w;
  }

  /// Sends one write now; if offline, queues it. Returns the response body or null if queued.
  Future<Map<String, dynamic>?> send(String method, String path, [Map<String, dynamic>? body]) async {
    final key = const Uuid().v4();
    try {
      final res = await _dio.request<Map<String, dynamic>>(path,
          data: body, options: Options(method: method, headers: {'Idempotency-Key': key}));
      return res.data;
    } catch (e) {
      final err = ApiException.from(e);
      if (!err.isOffline) rethrow;
      await enqueue(method, path, body);
      return null;
    }
  }

  Future<void> flush() async {
    if (_flushing) return;
    _flushing = true;
    try {
      final box = await _open();
      final keys = box.keys.cast<String>().toList()..sort();
      for (final k in keys) {
        final w = PendingWrite.fromJson(jsonDecode(box.get(k)!) as Map<String, dynamic>);
        try {
          await _dio.request<dynamic>(w.path,
              data: w.body, options: Options(method: w.method, headers: {'Idempotency-Key': w.id}));
        } catch (e) {
          final err = ApiException.from(e);
          if (err.isOffline || err.status == 429 || (err.status ?? 0) >= 500) return; // retry later
          if (!(w.method == 'DELETE' && err.status == 404)) _failures.add((w, err));
        }
        await box.delete(k);
        pending.value = box.length;
      }
    } finally {
      _flushing = false;
    }
  }
}

final outboxProvider = Provider<Outbox>((ref) {
  final outbox = Outbox(ref.watch(dioProvider));
  // Replay whenever connectivity comes back (and once at startup).
  ref.listen<AsyncValue<bool>>(onlineProvider, (_, next) {
    if (next.value == true) unawaited(outbox.flush());
  }, fireImmediately: true);
  return outbox;
});
