import 'package:flutter_form_builder/flutter_form_builder.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:form_builder_validators/form_builder_validators.dart';
import 'package:go_router/go_router.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../app/theme/app_theme.dart';
import '../../../core/config/env.dart';
import '../../../core/network/api_exception.dart';
import '../../../l10n/gen/app_localizations.dart';
import '../../../shared/widgets/paged_list_view.dart' show ContentWidth, showError;
import '../application/auth_controller.dart';
import 'verify_code_page.dart';

/// Password sign-up. Consent UX follows GDPR: terms acceptance is required
/// (contract), marketing is a separate, unticked, optional box (consent).
class RegisterPage extends ConsumerStatefulWidget {
  const RegisterPage({super.key});

  @override
  ConsumerState<RegisterPage> createState() => _RegisterPageState();
}

class _RegisterPageState extends ConsumerState<RegisterPage> {
  final _form = GlobalKey<FormBuilderState>();
  bool _busy = false;

  Future<void> _submit() async {
    if (!(_form.currentState?.saveAndValidate() ?? false)) return;
    final v = _form.currentState!.value;
    setState(() => _busy = true);
    try {
      await ref.read(authControllerProvider.notifier).register(
            email: v['email'] as String,
            password: v['password'] as String,
            name: (v['name'] as String?) ?? '',
            marketing: v['marketing'] as bool? ?? false,
            locale: Localizations.localeOf(context).languageCode,
          );
      if (mounted) {
        await context.push('/verify', extra: VerifyArgs(channel: 'email', destination: v['email'] as String, purpose: 'verify_email'));
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      e.fields.forEach((k, msg) => _form.currentState?.fields[k]?.invalidate(msg));
      showError(context, e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l.createAccount)),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(Tokens.lg),
        child: ContentWidth(
          child: AutofillGroup(
            child: FormBuilder(
              key: _form,
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                FormBuilderTextField(
                  name: 'name',
                  autofillHints: const [AutofillHints.name],
                  decoration: InputDecoration(labelText: l.name),
                  validator: FormBuilderValidators.maxLength(100),
                ),
                const SizedBox(height: Tokens.md),
                FormBuilderTextField(
                  name: 'email',
                  keyboardType: TextInputType.emailAddress,
                  autofillHints: const [AutofillHints.email],
                  decoration: InputDecoration(labelText: l.email),
                  validator: FormBuilderValidators.compose([FormBuilderValidators.required(), FormBuilderValidators.email()]),
                ),
                const SizedBox(height: Tokens.md),
                FormBuilderTextField(
                  name: 'password',
                  obscureText: true,
                  autofillHints: const [AutofillHints.newPassword],
                  decoration: InputDecoration(labelText: l.password, helperText: 'At least 10 characters. A passphrase works great.'),
                  validator: FormBuilderValidators.compose([FormBuilderValidators.required(), FormBuilderValidators.minLength(10)]),
                ),
                const SizedBox(height: Tokens.md),
                FormBuilderCheckbox(
                  name: 'terms',
                  initialValue: false,
                  title: Text(l.acceptTerms),
                  validator: FormBuilderValidators.equal(true, errorText: 'Required to create an account'),
                ),
                Wrap(spacing: Tokens.sm, children: [
                  TextButton(onPressed: () => launchUrl(Uri.parse(Env.termsUrl)), child: const Text('Terms')),
                  TextButton(onPressed: () => launchUrl(Uri.parse(Env.privacyUrl)), child: const Text('Privacy Policy')),
                ]),
                FormBuilderCheckbox(name: 'marketing', initialValue: false, title: Text(l.marketingOptIn)),
                const SizedBox(height: Tokens.lg),
                FilledButton(onPressed: _busy ? null : _submit, child: Text(l.createAccount)),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
