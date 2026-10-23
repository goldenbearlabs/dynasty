// Shows the signed-in account state before its General Manager joins a league.

import 'package:flutter/material.dart';

import '../data/session_controller.dart';

class AccountLobbyScreen extends StatefulWidget {
  const AccountLobbyScreen({super.key, required this.session});
  final SessionController session;

  @override
  State<AccountLobbyScreen> createState() => _AccountLobbyScreenState();
}

class _AccountLobbyScreenState extends State<AccountLobbyScreen> {
  String? message;
  final invite = TextEditingController();
  bool busy = false;

  @override
  void dispose() {
    invite.dispose();
    super.dispose();
  }

  Future<void> acceptInvite() async {
    setState(() {
      busy = true;
      message = null;
    });
    try {
      await widget.session.acceptInvite(invite.text);
    } catch (error) {
      if (mounted) setState(() => message = '$error');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> resend() async {
    try {
      await widget.session.api.resendVerification(
        widget.session.session!.email,
      );
      setState(
        () => message = 'If verification is needed, a new link has been sent.',
      );
    } catch (error) {
      setState(() => message = '$error');
    }
  }

  @override
  Widget build(BuildContext context) {
    final account = widget.session.session!;
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 560),
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                Text(
                  'Mosaic Fantasy',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 32),
                Text(
                  'My Leagues',
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 12),
                const Text(
                  'You are signed in, but you have not joined a league yet.',
                ),
                const SizedBox(height: 12),
                Text(account.email),
                if (!account.verified) ...[
                  const SizedBox(height: 20),
                  const Text(
                    'Verify your email before creating or joining a league.',
                  ),
                  TextButton(
                    onPressed: resend,
                    child: const Text('Resend verification email'),
                  ),
                ],
                if (account.verified) ...[
                  const SizedBox(height: 20),
                  const Text(
                    'Have a commissioner invite for a prepared franchise? Paste it below.',
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: invite,
                    decoration: const InputDecoration(
                      labelText: 'Invite link',
                      border: OutlineInputBorder(),
                    ),
                  ),
                  const SizedBox(height: 12),
                  FilledButton(
                    onPressed: busy ? null : acceptInvite,
                    child: const Text('Claim franchise'),
                  ),
                  const SizedBox(height: 20),
                  const Text(
                    'Creating a new league and joining with a shared code are coming in the next chunk.',
                  ),
                ],
                if (message != null) ...[
                  const SizedBox(height: 12),
                  Text(message!),
                ],
                const SizedBox(height: 24),
                OutlinedButton(
                  onPressed: widget.session.logout,
                  child: const Text('Sign out'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
