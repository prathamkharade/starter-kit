import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../network/api_exception.dart';

/// Mirrors the backend list contract (backend/internal/platform/listquery):
/// ?q=&filter[x]=&filter[x][gte]=&sort=-field&limit=&cursor=
class ListQuery {
  const ListQuery({this.q = '', this.filters = const {}, this.sort, this.limit = 20});

  final String q;
  final Map<String, String> filters; // 'archived' → 'false', 'tags][in' → 'a,b'
  final String? sort;
  final int limit;

  ListQuery copyWith({String? q, Map<String, String>? filters, String? sort, int? limit}) =>
      ListQuery(q: q ?? this.q, filters: filters ?? this.filters, sort: sort ?? this.sort, limit: limit ?? this.limit);

  Map<String, dynamic> toParams([String? cursor]) => {
        if (q.trim().isNotEmpty) 'q': q.trim(),
        for (final f in filters.entries)
          if (f.value.isNotEmpty) 'filter[${f.key}]': f.value,
        if (sort != null) 'sort': sort,
        'limit': limit,
        'cursor': ?cursor,
      };

  /// Stable cache key for offline storage of the first page.
  String get cacheKey => toParams().entries.map((e) => '${e.key}=${e.value}').join('&');

  @override
  bool operator ==(Object other) => other is ListQuery && other.cacheKey == cacheKey;
  @override
  int get hashCode => cacheKey.hashCode;
}

class PageData<T> {
  const PageData(this.items, {this.nextCursor, this.hasMore = false, this.fromCache = false});

  final List<T> items;
  final String? nextCursor;
  final bool hasMore;
  final bool fromCache;

  factory PageData.fromJson(Map<String, dynamic> json, T Function(Map<String, dynamic>) item, {bool fromCache = false}) =>
      PageData(
        (json['items'] as List).map((e) => item((e as Map).cast<String, dynamic>())).toList(),
        nextCursor: json['next_cursor'] as String?,
        hasMore: json['has_more'] as bool? ?? false,
        fromCache: fromCache,
      );
}

class PagedState<T> {
  const PagedState({
    required this.query,
    this.items = const [],
    this.loading = false,
    this.loadingMore = false,
    this.hasMore = true,
    this.cursor,
    this.error,
    this.stale = false,
  });

  final ListQuery query;
  final List<T> items;
  final bool loading;
  final bool loadingMore;
  final bool hasMore;
  final String? cursor;
  final ApiException? error;
  final bool stale; // showing cached data because we're offline

  PagedState<T> copyWith({
    ListQuery? query,
    List<T>? items,
    bool? loading,
    bool? loadingMore,
    bool? hasMore,
    String? cursor,
    ApiException? error,
    bool clearError = false,
    bool? stale,
  }) =>
      PagedState(
        query: query ?? this.query,
        items: items ?? this.items,
        loading: loading ?? this.loading,
        loadingMore: loadingMore ?? this.loadingMore,
        hasMore: hasMore ?? this.hasMore,
        cursor: cursor ?? this.cursor,
        error: clearError ? null : (error ?? this.error),
        stale: stale ?? this.stale,
      );
}

/// Extend this for any list screen: implement [fetch] (and optionally [initialQuery]).
/// Handles first load, refresh, infinite scroll, search/filter changes and
/// discards responses that arrive for an outdated query.
abstract class PagedNotifier<T> extends Notifier<PagedState<T>> {
  ListQuery get initialQuery => const ListQuery();

  Future<PageData<T>> fetch(ListQuery query, String? cursor);

  @override
  PagedState<T> build() {
    Future.microtask(refresh);
    return PagedState<T>(query: initialQuery, loading: true);
  }

  Future<void> refresh() => _load(state.query, reset: true);

  Future<void> setQuery(ListQuery q) {
    if (q == state.query && state.items.isNotEmpty) return Future.value();
    return _load(q, reset: true);
  }

  Future<void> loadMore() async {
    if (state.loading || state.loadingMore || !state.hasMore || state.stale) return;
    await _load(state.query, reset: false);
  }

  Future<void> _load(ListQuery q, {required bool reset}) async {
    state = reset
        ? state.copyWith(query: q, loading: true, clearError: true)
        : state.copyWith(loadingMore: true, clearError: true);
    try {
      final page = await fetch(q, reset ? null : state.cursor);
      if (!ref.mounted || state.query != q) return; // stale response
      state = PagedState<T>(
        query: q,
        items: reset ? page.items : [...state.items, ...page.items],
        hasMore: page.hasMore,
        cursor: page.nextCursor,
        stale: page.fromCache,
      );
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(loading: false, loadingMore: false, error: ApiException.from(e));
    }
  }

  /// Optimistic local edits (after create/update/delete) without refetching.
  void upsertLocal(T item, bool Function(T a, T b) same) {
    final i = state.items.indexWhere((x) => same(x, item));
    final list = [...state.items];
    i >= 0 ? list[i] = item : list.insert(0, item);
    state = state.copyWith(items: list);
  }

  void removeLocal(bool Function(T) test) => state = state.copyWith(items: state.items.where((x) => !test(x)).toList());
}
