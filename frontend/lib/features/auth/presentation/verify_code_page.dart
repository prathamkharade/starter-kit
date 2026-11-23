import 'package:flutter_form_builder/flutter_form_builder.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:form_builder_validators/form_builder_validators.dart';
import 'package:material_ui/material_ui.dart';

import '../../../app/theme/app_theme.dart';
import '../../../core/network/api_exception.dart';
import '../../../l10n/gen/app_localizations.dart';
import '../../../shared/widgets/paged_list_view.dart' show ContentWidth, showError;
import '../application/auth_controller.dart';

class VerifyArgs {
  const VerifyArgs({required this.channel, required this.destination, required this.purpose});
  final String channel; // email | sms
  final String destination;
  final String purpose; // login | verify_email | reset
  static const empty = VerifyArgs(channel: 'email', destination: '', purpose: 'login');
}

class VerifyCodePage extends ConsumerStatefulWidget {
  const VerifyCodePage({super.key, required this.args});
  final VerifyArgs args;

  @override
  ConsumerState<VerifyCodePage> createState() => _VerifyCodePageState();
}

class _VerifyCodePageState extends ConsumerState<VerifyCodePage> {
  final _form = GlobalKey<FormBuilderState>();
  bool _busy = false;

  Future<void> _submit() async {
    if (!(_form.currentState?.saveAndValidate() ?? false)) return;
    final v = _form.currentState!.value;
    final code = (v['code'] as String).trim();
    final a = widget.args;
    final auth = ref.read(authControllerProvider.notifier);
    setState(() => _busy = true);
    try {
      switch (a.purpose) {
        case 'verify_email':
          await auth.verifyEmail(a.destination, code);
        case 'reset':
          await auth.resetPassword(a.destination, code, v['new_password'] as String);
        default:
          await auth.verifyCode(a.channel, a.destination, code);
      }
      // Router redirect takes over once signed in.
    } on ApiException catch (e) {
      if (mounted) showError(context, e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _resend() async {
    final a = widget.args;
    final auth = ref.read(authControllerProvider.notifier);
    try {
      a.purpose == 'reset' ? await auth.forgotPassword(a.destination) : await auth.requestCode(a.channel, a.destination);
      if (mounted) showError(context, 'A new code is on its way.');
    } on ApiException catch (e) {
      if (mounted) showError(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final a = widget.args;
    return Scaffold(
      appBar: AppBar(),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(Tokens.lg),
        child: ContentWidth(
          child: FormBuilder(
            key: _form,
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text(l.enterCode, style: Theme.of(context).textTheme.headlineSmall),
              const SizedBox(height: Tokens.sm),
              Text(l.codeSentTo(a.destination)),
              const SizedBox(height: Tokens.lg),
              FormBuilderTextField(
                name: 'code',
                autofocus: true,
                keyboardType: TextInputType.number,
                autofillHints: const [AutofillHints.oneTimeCode], // SMS/email code autofill on iOS & Android
                maxLength: 6,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.headlineSmall?.copyWith(letterSpacing: 8),
                decoration: const InputDecoration(counterText: ''),
                validator: FormBuilderValidators.compose([
                  FormBuilderValidators.required(),
                  FormBuilderValidators.numeric(),
                  FormBuilderValidators.equalLength(6),
                ]),
                onChanged: (v) {
                  if (v?.length == 6 && a.purpose != 'reset') _submit(); // auto-submit
                },
              ),
              if (a.purpose == 'reset') ...[
                const SizedBox(height: Tokens.md),
                FormBuilderTextField(
                  name: 'new_password',
                  obscureText: true,
                  autofillHints: const [AutofillHints.newPassword],
                  decoration: InputDecoration(labelText: l.newPassword, helperText: 'At least 10 characters'),
                  validator: FormBuilderValidators.compose([FormBuilderValidators.required(), FormBuilderValidators.minLength(10)]),
                ),
              ],
              const SizedBox(height: Tokens.lg),
              FilledButton(onPressed: _busy ? null : _submit, child: Text(l.continueButton)),
              TextButton(onPressed: _busy ? null : _resend, child: Text(l.resendCode)),
            ]),
          ),
        ),
      ),
    );
  }
}
