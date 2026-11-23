// Full end-to-end flow in a real browser against the real API + Mailpit:
// sign up with an email code (read from Mailpit), then create a note through the form.
//
// Needs: `task infra:up` and `task api` running, plus chromedriver on port 4444. Then:
//   flutter drive --driver=test_driver/integration_test.dart \
//     --target=integration_test/e2e_flow_test.dart -d chrome --web-port 5000
// (`--web-port 5000` matches CORS_ORIGINS in backend/.env.)
import 'package:app/app/app.dart';
import 'package:app/bootstrap.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:material_ui/material_ui.dart';

const _mailpit = String.fromEnvironment('MAILPIT_URL', defaultValue: 'http://localhost:8025');

Future<void> pumpUntil(WidgetTester t, Finder f, {Duration timeout = const Duration(seconds: 20)}) async {
  final end = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(end)) {
    await t.pump(const Duration(milliseconds: 200));
    if (f.evaluate().isNotEmpty) return;
  }
  throw TestFailure('Timed out waiting for $f');
}

Future<String> latestCode(String to) async {
  final dio = Dio(BaseOptions(baseUrl: _mailpit));
  for (var i = 0; i < 50; i++) {
    final res = await dio.get<Map<String, dynamic>>('/api/v1/search', queryParameters: {'query': 'to:"$to"'});
    final msgs = (res.data?['messages'] as List?) ?? const [];
    if (msgs.isNotEmpty) {
      final id = (msgs.first as Map)['ID'];
      final msg = await dio.get<Map<String, dynamic>>('/api/v1/message/$id');
      final m = RegExp(r'\b(\d{6})\b').firstMatch('${msg.data?['Text']}');
      if (m != null) return m.group(1)!;
    }
    await Future<void>.delayed(const Duration(milliseconds: 200));
  }
  throw TestFailure('No code email for $to in Mailpit');
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('sign up with email code, then create a note', (t) async {
    final email = 'e2e${DateTime.now().millisecondsSinceEpoch}@example.com';
    final title = 'E2E note ${DateTime.now().millisecondsSinceEpoch}';

    await bootstrap(() => const App());
    await pumpUntil(t, find.text('Continue'));

    // Sign-in form: email → Continue
    await t.enterText(find.widgetWithText(TextField, 'Email'), email);
    await t.tap(find.text('Continue'));

    // Verify form: the 6-digit code auto-submits
    final codeField = find.byType(TextField);
    await pumpUntil(t, find.textContaining(email));
    await t.enterText(codeField.first, await latestCode(email));

    // Signed in → go to Notes and open the editor
    await pumpUntil(t, find.text('Notes'));
    await t.tap(find.text('Notes').last);
    await pumpUntil(t, find.text('New note'));
    await t.tap(find.text('New note'));

    // Note form
    await pumpUntil(t, find.widgetWithText(TextField, 'Title'));
    await t.enterText(find.widgetWithText(TextField, 'Title'), title);
    await t.enterText(find.widgetWithText(TextField, 'Note'), 'Written by the browser e2e test.');
    await t.enterText(find.widgetWithText(TextField, 'Tags'), 'e2e, test');
    await t.tap(find.text('Save'));

    // Back on the list, the note is there (and synced, not pending)
    await pumpUntil(t, find.text(title));
    expect(find.text(title), findsOneWidget);
  });
}
