// Lists the signed-in account's leagues and keeps unfinished setups separate from playable leagues.
// New accounts can verify email and accept an existing commissioner invite here.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/session_controller.dart';
import 'load_view.dart';

class LeaguesScreen extends StatefulWidget {
  const LeaguesScreen({
    super.key,
    required this.api,
    required this.session,
    required this.onOpen,
  });

  final ApiClient api;
  final SessionController session;
  final void Function(MyLeague league, Dynasty dynasty) onOpen;

  @override
  State<LeaguesScreen> createState() => _LeaguesScreenState();
}

class _LeaguesScreenState extends State<LeaguesScreen> {
  late Future<List<MyLeague>> future = widget.api.myLeagues();
  final invite = TextEditingController();
  String? busyLeague;
  String? notice;
  bool inviteBusy = false;

  @override
  void dispose() {
    invite.dispose();
    super.dispose();
  }

  Future<void> refresh() async {
    final next = widget.api.myLeagues();
    setState(() {
      future = next;
    });
    try {
      await next;
    } catch (_) {}
  }

  Future<void> open(MyLeague league) async {
    if (!league.ready || busyLeague != null) return;
    setState(() {
      busyLeague = league.id;
      notice = null;
    });
    try {
      final dynasty = await widget.api.dynasty();
      if (dynasty.id.isEmpty || dynasty.id != league.id) {
        throw const ApiException(
          'This league is saved, but its mobile gameplay is not available yet.',
        );
      }
      if (mounted) widget.onOpen(league, dynasty);
    } catch (error) {
      if (mounted) setState(() => notice = '$error');
    } finally {
      if (mounted) setState(() => busyLeague = null);
    }
  }

  Future<void> acceptInvite() async {
    setState(() {
      inviteBusy = true;
      notice = null;
    });
    try {
      await widget.session.acceptInvite(invite.text);
      invite.clear();
    } catch (error) {
      if (mounted) setState(() => notice = '$error');
    } finally {
      if (mounted) setState(() => inviteBusy = false);
    }
  }

  Future<void> resendVerification() async {
    setState(() {
      inviteBusy = true;
      notice = null;
    });
    try {
      await widget.api.resendVerification(widget.session.session!.email);
      if (mounted) {
        setState(
          () => notice = 'If verification is needed, a new link has been sent.',
        );
      }
    } catch (error) {
      if (mounted) setState(() => notice = '$error');
    } finally {
      if (mounted) setState(() => inviteBusy = false);
    }
  }

  @override
  Widget build(BuildContext context) => LoadView(
    future: future,
    onRetry: refresh,
    builder: (leagues) => RefreshIndicator(
      onRefresh: refresh,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(20, 20, 20, 100),
        children: [
          Text(
            'MY LEAGUES',
            style: TextStyle(
              color: Theme.of(context).colorScheme.primary,
              fontSize: 12,
              fontWeight: FontWeight.w800,
              letterSpacing: 2,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            'Choose a league',
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          const SizedBox(height: 8),
          Text(
            widget.session.session?.email ?? '',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 24),
          if (leagues.isEmpty)
            const Card(
              child: Padding(
                padding: EdgeInsets.all(20),
                child: Text('You have not joined a league yet.'),
              ),
            ),
          for (final league in leagues) ...[
            Card(
              child: ListTile(
                contentPadding: const EdgeInsets.all(16),
                leading: Icon(
                  league.ready
                      ? Icons.emoji_events_outlined
                      : Icons.schedule_outlined,
                  color: league.ready ? const Color(0xfff0c050) : null,
                ),
                title: Text(league.name),
                subtitle: Text(
                  '${league.sports.map((sport) => sport.toUpperCase()).join(' · ')}\n'
                  '${league.franchiseName}${league.ready ? '' : ' · Setup pending'}',
                ),
                isThreeLine: true,
                trailing: busyLeague == league.id
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : league.ready
                    ? const Icon(Icons.chevron_right)
                    : const Icon(Icons.lock_clock_outlined),
                onTap: league.ready ? () => open(league) : null,
              ),
            ),
            const SizedBox(height: 8),
          ],
          if (notice != null) ...[
            const SizedBox(height: 12),
            Text(
              notice!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ],
          if (!leagues.any((league) => league.ready)) ...[
            const SizedBox(height: 20),
            if (widget.session.session?.verified == false) ...[
              const Text(
                'Verify your email, then ask a commissioner for an invite link to join a league.',
              ),
              const SizedBox(height: 8),
              OutlinedButton(
                onPressed: inviteBusy ? null : resendVerification,
                child: const Text('Resend verification email'),
              ),
            ] else ...[
              const Text('Have a commissioner invite? Paste its link to join.'),
              const SizedBox(height: 12),
              TextField(
                controller: invite,
                keyboardType: TextInputType.url,
                decoration: const InputDecoration(
                  labelText: 'Invite link',
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 12),
              FilledButton(
                onPressed: inviteBusy ? null : acceptInvite,
                child: const Text('Join league'),
              ),
            ],
          ],
        ],
      ),
    ),
  );
}
