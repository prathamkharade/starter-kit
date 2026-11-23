import 'package:app/features/auth/presentation/sign_in_page.dart';
import 'package:app/l10n/gen/app_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:form_builder_validators/form_builder_validators.dart';
import 'package:material_ui/material_ui.dart';

Widget wrap(Widget child) => ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          ...AppLocalizations.localizationsDelegates,
          FormBuilderLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: child,
      ),
    );

void main() {
  testWidgets('sign-in validates the email before calling the API', (tester) async {
    await tester.pumpWidget(wrap(const SignInPage()));
    await tester.enterText(find.byType(TextField).first, 'not-an-email');
    await tester.tap(find.text('Continue'));
    await tester.pumpAndSettle();
    expect(find.textContaining('email', findRichText: true), findsWidgets);
    expect(find.byType(SnackBar), findsNothing); // no network call happened
  });

  testWidgets('can switch to password mode', (tester) async {
    await tester.pumpWidget(wrap(const SignInPage()));
    await tester.tap(find.text('Use a password instead'));
    await tester.pumpAndSettle();
    expect(find.text('Password'), findsOneWidget);
    expect(find.text('Forgot password?'), findsOneWidget);
  });
}
