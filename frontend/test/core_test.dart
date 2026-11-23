import 'package:app/core/network/api_exception.dart';
import 'package:app/core/pagination/paged.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('ListQuery', () {
    test('serialises to the backend contract', () {
      const q = ListQuery(q: ' milk ', filters: {'archived': 'false', 'tags][in': 'a,b'}, sort: '-updated_at', limit: 10);
      expect(q.toParams('abc'), {
        'q': 'milk',
        'filter[archived]': 'false',
        'filter[tags][in]': 'a,b',
        'sort': '-updated_at',
        'limit': 10,
        'cursor': 'abc',
      });
    });

    test('equality is by parameters', () {
      expect(const ListQuery(q: 'a'), const ListQuery(q: 'a'));
      expect(const ListQuery(q: 'a') == const ListQuery(q: 'b'), isFalse);
    });
  });

  group('ApiException', () {
    test('parses the error envelope', () {
      final err = DioException(
        requestOptions: RequestOptions(path: '/x'),
        response: Response(
          requestOptions: RequestOptions(path: '/x'),
          statusCode: 422,
          data: {
            'error': {'code': 'validation_failed', 'message': 'Bad', 'fields': {'email': 'email'}},
          },
        ),
        type: DioExceptionType.badResponse,
      );
      final e = ApiException.from(err);
      expect(e.code, 'validation_failed');
      expect(e.status, 422);
      expect(e.fields['email'], 'email');
    });

    test('maps connection errors to offline', () {
      final e = ApiException.from(DioException(requestOptions: RequestOptions(path: '/x'), type: DioExceptionType.connectionError));
      expect(e.isOffline, isTrue);
    });
  });
}
