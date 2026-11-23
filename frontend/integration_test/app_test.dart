// Full-app smoke test on a real device/emulator/browser:
//   flutter test integration_test                        (Android/iOS/desktop)
//   flutter drive --driver=test_driver/integration_test.dart \
//     --target=integration_test/app_test.dart -d chrome  (web; needs chromedriver)
// The backend does not need to be running: an unreachable API must land on sign-in.
import 'package:app/app/app.dart';
import 'package:app/bootstrap.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('boots to the sign-in screen', (tester) async {
    await bootstrap(() => const App());
    await tester.pumpAndSettle(const Duration(seconds: 3));
    expect(find.byType(TextField), findsWidgets);
    expect(find.text('Continue'), findsOneWidget);
  });
}
