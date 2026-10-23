// Lets managers sign in or claim a commissioner invite without leaving the app.

import 'package:flutter/material.dart';

import '../data/session_controller.dart';

class AuthScreen extends StatefulWidget {
  const AuthScreen({super.key, required this.session});
  final SessionController session;

  @override
  State<AuthScreen> createState() => _AuthScreenState();
}

class _AuthScreenState extends State<AuthScreen> {
  final form = GlobalKey<FormState>();
  final email = TextEditingController();
  final password = TextEditingController();
  final invite = TextEditingController();
  bool claiming = false;
  bool registering = false;
  bool busy = false;
  String? error;

  @override
  void dispose() {
    email.dispose();
    password.dispose();
    invite.dispose();
    super.dispose();
  }

  Future<void> submit() async {
    if (!form.currentState!.validate()) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      if (claiming) {
        await widget.session.claim(invite.text, email.text, password.text);
      } else if (registering) {
        final sent = await widget.session.register(email.text, password.text);
        if (mounted && !sent) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text(
                'Account created, but email delivery failed. Resend it from Leagues.',
              ),
            ),
          );
        }
      } else {
        await widget.session.signIn(email.text, password.text);
      }
      if (mounted && Navigator.of(context).canPop()) {
        Navigator.of(context).pop();
      }
    } catch (failure) {
      if (mounted) setState(() => error = '$failure');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> resetPassword() async {
    if (email.text.trim().isEmpty) {
      setState(() => error = 'Enter your email first.');
      return;
    }
    try {
      await widget.session.api.requestPasswordReset(email.text.trim());
      if (mounted) {
        setState(
          () => error =
              'If that email has an account, a reset link has been sent.',
        );
      }
    } catch (failure) {
      if (mounted) setState(() => error = '$failure');
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(
        claiming
            ? 'Claim your invite'
            : registering
            ? 'Create account'
            : 'Sign in',
      ),
    ),
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 440),
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const Icon(Icons.shield_outlined, size: 42),
            const SizedBox(height: 16),
            Text(
              claiming
                  ? 'Join your franchise'
                  : registering
                  ? 'Join Mosaic Fantasy'
                  : 'Welcome back',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            const SizedBox(height: 8),
            Text(
              claiming
                  ? 'Paste the invite link your commissioner sent you.'
                  : registering
                  ? 'Create an account, then verify your email to join a league.'
                  : 'Sign in to see your leagues and franchises.',
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 28),
            Form(
              key: form,
              child: Column(
                children: [
                  if (claiming) ...[
                    TextFormField(
                      controller: invite,
                      decoration: const InputDecoration(
                        labelText: 'Invite link',
                        border: OutlineInputBorder(),
                      ),
                      keyboardType: TextInputType.url,
                      validator: (value) =>
                          value == null || value.trim().isEmpty
                          ? 'Paste your invite link.'
                          : null,
                    ),
                    const SizedBox(height: 14),
                  ],
                  TextFormField(
                    controller: email,
                    decoration: const InputDecoration(
                      labelText: 'Email',
                      border: OutlineInputBorder(),
                    ),
                    keyboardType: TextInputType.emailAddress,
                    autofillHints: const [AutofillHints.email],
                    validator: (value) => value == null || value.trim().isEmpty
                        ? 'Enter your email.'
                        : null,
                  ),
                  const SizedBox(height: 14),
                  TextFormField(
                    controller: password,
                    decoration: const InputDecoration(
                      labelText: 'Password',
                      border: OutlineInputBorder(),
                    ),
                    obscureText: true,
                    autofillHints: [
                      claiming || registering
                          ? AutofillHints.newPassword
                          : AutofillHints.password,
                    ],
                    validator: (value) => value == null || value.isEmpty
                        ? 'Enter your password.'
                        : null,
                  ),
                ],
              ),
            ),
            if (error != null)
              Padding(
                padding: const EdgeInsets.only(top: 16),
                child: Text(
                  error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            const SizedBox(height: 20),
            FilledButton(
              onPressed: busy ? null : submit,
              child: Text(
                busy
                    ? 'Please wait…'
                    : claiming
                    ? 'Claim invite'
                    : registering
                    ? 'Create account'
                    : 'Sign in',
              ),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: busy
                  ? null
                  : () => setState(() {
                      claiming = !claiming;
                      registering = false;
                      error = null;
                    }),
              child: Text(
                claiming
                    ? 'Already have an account? Sign in'
                    : 'Have an invite link? Claim a franchise',
              ),
            ),
            TextButton(
              onPressed: busy
                  ? null
                  : () => setState(() {
                      registering = !registering;
                      claiming = false;
                      error = null;
                    }),
              child: Text(
                registering
                    ? 'Already have an account? Sign in'
                    : 'New here? Create an account',
              ),
            ),
            if (!claiming && !registering)
              TextButton(
                onPressed: busy ? null : resetPassword,
                child: const Text('Forgot your password?'),
              ),
          ],
        ),
      ),
    ),
  );
}
