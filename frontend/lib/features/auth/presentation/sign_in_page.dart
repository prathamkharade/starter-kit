import 'package:flutter_form_builder/flutter_form_builder.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:form_builder_validators/form_builder_validators.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';

import '../../../app/theme/app_theme.dart';
import '../../../core/network/api_exception.dart';
import '../../../l10n/gen/app_localizations.dart';
import '../../../shared/widgets/paged_list_view.dart' show ContentWidth, showError;
import '../application/auth_controller.dart';
import 'verify_code_page.dart';

/// Frictionless entry: one field → code → in. Same screen signs up new users
/// (SIGNUP_VIA_OTP=true). Password sign-in is one tap away for those who prefer it.
class SignInPage extends ConsumerStatefulWidget {
  const SignInPage({super.key});

  @override
  ConsumerState<SignInPage> createState() => _SignInPageState();
}

class _SignInPageState extends ConsumerState<SignInPage> {
  final _form = GlobalKey<FormBuilderState>();
  bool _phone = false;
  bool _password = false;
  bool _busy = false;

  Future<void> _submit() async {
    if (!(_form.currentState?.saveAndValidate() ?? false)) return;
    final v = _form.currentState!.value;
    final auth = ref.read(authControllerProvider.notifier);
    setState(() => _busy = true);
    try {
      if (_password) {
        await auth.signInWithPassword(v['email'] as String, v['password'] as String);
      } else {
        final channel = _phone ? 'sms' : 'email';
        final dest = (_phone ? v['phone'] : v['email']) as String;
        await auth.requestCode(channel, dest);
        if (mounted) await context.push('/verify', extra: VerifyArgs(channel: channel, destination: dest, purpose: 'login'));
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      if (e.code == 'email_not_verified') {
        await context.push('/verify', extra: VerifyArgs(channel: 'email', destination: v['email'] as String, purpose: 'verify_email'));
      } else {
        showError(context, e.message);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _forgot() async {
    final email = _form.currentState?.fields['email']?.value as String?;
    if (email == null || email.isEmpty) {
      _form.currentState?.fields['email']?.invalidate('Enter your email first');
      return;
    }
    await ref.read(authControllerProvider.notifier).forgotPassword(email);
    if (mounted) await context.push('/verify', extra: VerifyArgs(channel: 'email', destination: email, purpose: 'reset'));
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final t = Theme.of(context).textTheme;
    return Scaffold(
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(Tokens.lg),
          child: ContentWidth(
            child: AutofillGroup(
              child: FormBuilder(
                key: _form,
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  const SizedBox(height: Tokens.xxl),
                  Text(l.welcome, style: t.headlineMedium),
                  const SizedBox(height: Tokens.sm),
                  Text(l.signInSubtitle, style: t.bodyLarge),
                  const SizedBox(height: Tokens.xl),
                  if (!_password)
                    SegmentedButton<bool>(
                      segments: [
                        ButtonSegment(value: false, label: Text(l.email), icon: const Icon(Icons.mail_outline)),
                        ButtonSegment(value: true, label: Text(l.phone), icon: const Icon(Icons.phone_iphone)),
                      ],
                      selected: {_phone},
                      onSelectionChanged: (s) => setState(() => _phone = s.first),
                    ),
                  const SizedBox(height: Tokens.md),
                  if (_phone && !_password)
                    FormBuilderTextField(
                      name: 'phone',
                      keyboardType: TextInputType.phone,
                      autofillHints: const [AutofillHints.telephoneNumber],
                      decoration: InputDecoration(labelText: l.phone, hintText: '+49 170 1234567'),
                      validator: FormBuilderValidators.compose([
                        FormBuilderValidators.required(),
                        FormBuilderValidators.match(RegExp(r'^\+[1-9][0-9 ()\-]{6,20}$'), errorText: 'Include country code, e.g. +49…'),
                      ]),
                    )
                  else
                    FormBuilderTextField(
                      name: 'email',
                      keyboardType: TextInputType.emailAddress,
                      autofillHints: const [AutofillHints.email],
                      textInputAction: _password ? TextInputAction.next : TextInputAction.done,
                      decoration: InputDecoration(labelText: l.email),
                      validator: FormBuilderValidators.compose([FormBuilderValidators.required(), FormBuilderValidators.email()]),
                      onSubmitted: (_) => _password ? null : _submit(),
                    ),
                  if (_password) ...[
                    const SizedBox(height: Tokens.md),
                    FormBuilderTextField(
                      name: 'password',
                      obscureText: true,
                      autofillHints: const [AutofillHints.password],
                      decoration: InputDecoration(labelText: l.password),
                      validator: FormBuilderValidators.required(),
                      onSubmitted: (_) => _submit(),
                    ),
                    Align(
                      alignment: Alignment.centerRight,
                      child: TextButton(onPressed: _busy ? null : _forgot, child: Text(l.forgotPassword)),
                    ),
                  ],
                  const SizedBox(height: Tokens.lg),
                  FilledButton(
                    onPressed: _busy ? null : _submit,
                    child: _busy
                        ? const SizedBox.square(dimension: 22, child: CircularProgressIndicator(strokeWidth: 2.5))
                        : Text(_password ? l.signIn : l.continueButton),
                  ),
                  const SizedBox(height: Tokens.sm),
                  TextButton(
                    onPressed: () => setState(() {
                      _password = !_password;
                      _phone = false;
                    }),
                    child: Text(_password ? l.useCode : l.usePassword),
                  ),
                  if (_password) TextButton(onPressed: () => context.push('/register'), child: Text(l.createAccount)),
                  const SizedBox(height: Tokens.lg),
                  Text(l.termsNotice, style: t.bodySmall, textAlign: TextAlign.center),
                ]),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
