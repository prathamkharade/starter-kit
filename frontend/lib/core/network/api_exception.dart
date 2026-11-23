import 'package:dio/dio.dart';

/// Every failed API call surfaces as an ApiException. Switch on [code]
/// (stable, from the backend envelope) — never on [message].
class ApiException implements Exception {
  ApiException(this.code, this.message, {this.status, this.fields = const {}, this.body});

  final String code;
  final String message;
  final int? status;
  final Map<String, String> fields;
  final Map<String, dynamic>? body;

  bool get isOffline => code == 'offline';
  bool get isConflict => status == 409;

  factory ApiException.from(Object error) {
    if (error is ApiException) return error;
    if (error is DioException) {
      switch (error.type) {
        case DioExceptionType.connectionError:
        case DioExceptionType.connectionTimeout:
        case DioExceptionType.receiveTimeout:
        case DioExceptionType.sendTimeout:
          return ApiException('offline', 'You appear to be offline. We will retry automatically.');
        default:
          break;
      }
      final data = error.response?.data;
      if (data is Map && data['error'] is Map) {
        final e = (data['error'] as Map).cast<String, dynamic>();
        return ApiException(
          e['code'] as String? ?? 'error',
          e['message'] as String? ?? 'Something went wrong',
          status: error.response?.statusCode,
          fields: (e['fields'] as Map?)?.map((k, v) => MapEntry('$k', '$v')) ?? const {},
          body: data.cast<String, dynamic>(),
        );
      }
      return ApiException('error', 'Something went wrong', status: error.response?.statusCode);
    }
    return ApiException('error', error.toString());
  }

  @override
  String toString() => 'ApiException($code, $status): $message';
}
