// Presents a signed-in manager's playable leagues and separates saved setups.
// Invite claiming stays in a sheet; opening retains the dynasty-ID safety check.

import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/session_controller.dart';

const _surface = Color(0xff141822);
const _surfaceRaised = Color(0xff1a1f2c);
const _ink = Color(0xffeceef5);
const _inkSoft = Color(0xffa0a7ba);
const _rule = Color(0xff252b3a);
const _ruleStrong = Color(0xff343b4f);
const _brand = Color(0xff8499ff);

const _sportColors = <String, Color>{
  'cbb': Color(0xffff9547),
  'nba': Color(0xffa48bff),
  'wnba': Color(0xffff8fb3),
  'nhl': Color(0xff4fd1e0),
  'nfl': Color(0xff62d27a),
  'mlb': Color(0xffff7676),
};

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
  String? busyLeague;
  String? notice;
  bool noticeIsError = true;
  bool resendBusy = false;

  Future<void> refresh() async {
    final next = widget.api.myLeagues();
    setState(() {
      future = next;
    });
    try {
      await next;
    } catch (_) {
      // FutureBuilder shows the error and keeps the retry action available.
    }
  }

  Future<void> open(MyLeague league) async {
    if (!league.ready || busyLeague != null) return;
    setState(() {
      busyLeague = league.id;
      notice = null;
      noticeIsError = true;
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

  Future<void> resendVerification() async {
    setState(() {
      resendBusy = true;
      notice = null;
      noticeIsError = true;
    });
    try {
      await widget.api.resendVerification(widget.session.session!.email);
      if (mounted) {
        setState(() {
          notice = 'If verification is needed, a new link has been sent.';
          noticeIsError = false;
        });
      }
    } catch (error) {
      if (mounted) setState(() => notice = '$error');
    } finally {
      if (mounted) setState(() => resendBusy = false);
    }
  }

  void showInviteSheet() {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: _surfaceRaised,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => _InviteSheet(session: widget.session, onJoined: refresh),
    );
  }

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final horizontalPadding = constraints.maxWidth < 600 ? 20.0 : 32.0;
      final contentWidth = math.min(
        constraints.maxWidth - horizontalPadding * 2,
        960.0,
      );
      final columns = contentWidth >= 720 ? 2 : 1;
      final cardWidth = (contentWidth - (columns - 1) * 14) / columns;

      return FutureBuilder<List<MyLeague>>(
        future: future,
        builder: (context, snapshot) {
          final leagues = snapshot.data ?? const <MyLeague>[];
          final playable = leagues.where((league) => league.ready).toList();
          final loading = snapshot.connectionState != ConnectionState.done;

          return RefreshIndicator(
            onRefresh: refresh,
            color: _brand,
            backgroundColor: _surfaceRaised,
            child: ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: EdgeInsets.fromLTRB(
                horizontalPadding,
                12,
                horizontalPadding,
                100,
              ),
              children: [
                Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 960),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            IconButton(
                              tooltip: 'Open menu',
                              onPressed: () =>
                                  Scaffold.of(context).openDrawer(),
                              icon: const Icon(Icons.menu),
                              constraints: const BoxConstraints(
                                minWidth: 48,
                                minHeight: 48,
                              ),
                              padding: EdgeInsets.zero,
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Text(
                                'Leagues',
                                style: Theme.of(context)
                                    .textTheme
                                    .headlineMedium
                                    ?.copyWith(
                                      color: _ink,
                                      fontWeight: FontWeight.w800,
                                      letterSpacing: -0.6,
                                    ),
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 22),
                        if (notice != null) ...[
                          _MessagePanel(
                            message: notice!,
                            isError: noticeIsError,
                          ),
                          const SizedBox(height: 20),
                        ],
                        if (loading)
                          const _StatusPanel(
                            icon: Icons.hourglass_top_rounded,
                            title: 'Loading your leagues',
                            description: 'Checking your memberships…',
                            busy: true,
                          )
                        else if (snapshot.hasError)
                          _StatusPanel(
                            icon: Icons.wifi_off_rounded,
                            title: 'Could not load your leagues',
                            description: '${snapshot.error}',
                            action: FilledButton.icon(
                              onPressed: refresh,
                              icon: const Icon(Icons.refresh),
                              label: const Text('Try again'),
                            ),
                          )
                        else ...[
                          if (playable.isNotEmpty)
                            Wrap(
                              spacing: 14,
                              runSpacing: 14,
                              children: [
                                for (final league in playable)
                                  SizedBox(
                                    width: cardWidth,
                                    child: _leagueCard(league),
                                  ),
                              ],
                            ),
                          if (playable.isEmpty) ...[
                            _StatusPanel(
                              icon: widget.session.session?.verified == false
                                  ? Icons.mark_email_unread_outlined
                                  : Icons.groups_outlined,
                              title: widget.session.session?.verified == false
                                  ? 'Verify your email'
                                  : 'No leagues yet',
                              description:
                                  widget.session.session?.verified == false
                                  ? 'Verify your email, then ask a commissioner for an invite link to join a league.'
                                  : 'Ask a commissioner for an invite link to join a playable league.',
                              action: widget.session.session?.verified == false
                                  ? OutlinedButton.icon(
                                      onPressed: resendBusy
                                          ? null
                                          : resendVerification,
                                      icon: const Icon(Icons.outgoing_mail),
                                      label: const Text(
                                        'Resend verification email',
                                      ),
                                    )
                                  : FilledButton.icon(
                                      onPressed: showInviteSheet,
                                      icon: const Icon(Icons.add_link),
                                      label: const Text('Join a league'),
                                    ),
                            ),
                          ],
                        ],
                      ],
                    ),
                  ),
                ),
              ],
            ),
          );
        },
      );
    },
  );

  Widget _leagueCard(MyLeague league) {
    return Material(
      color: _surfaceRaised,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: const BorderSide(color: _ruleStrong),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () => open(league),
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: Text(
                      league.name,
                      style: Theme.of(context).textTheme.titleLarge
                          ?.copyWith(color: _ink, fontWeight: FontWeight.w800),
                    ),
                  ),
                  if (busyLeague == league.id)
                    const Padding(
                      padding: EdgeInsets.only(left: 12),
                      child: SizedBox.square(
                        dimension: 24,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  const Icon(Icons.shield_outlined, size: 17, color: _inkSoft),
                  const SizedBox(width: 7),
                  Expanded(
                    child: Text(
                      league.franchiseName,
                      style: const TextStyle(color: _inkSoft),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 18),
              Wrap(
                spacing: 12,
                runSpacing: 10,
                children: [
                  for (final sport in league.sports) _SportMarker(sport: sport),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _SportMarker extends StatelessWidget {
  const _SportMarker({required this.sport});

  final String sport;

  @override
  Widget build(BuildContext context) {
    final normalized = sport.toLowerCase();
    final color = _sportColors[normalized] ?? _inkSoft;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        DecoratedBox(
          key: Key('sport-dot-$normalized'),
          decoration: BoxDecoration(
            color: color,
            shape: BoxShape.circle,
            boxShadow: [
              BoxShadow(
                color: color.withValues(alpha: 0.35),
                blurRadius: 9,
                spreadRadius: 1,
                offset: const Offset(0, 1),
              ),
            ],
          ),
          child: const SizedBox.square(dimension: 8),
        ),
        const SizedBox(width: 8),
        Text(
          normalized.toUpperCase(),
          style: const TextStyle(
            color: _ink,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.3,
          ),
        ),
      ],
    );
  }
}

class _StatusPanel extends StatelessWidget {
  const _StatusPanel({
    required this.icon,
    required this.title,
    required this.description,
    this.action,
    this.busy = false,
  });

  final IconData icon;
  final String title;
  final String description;
  final Widget? action;
  final bool busy;

  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(22),
    decoration: BoxDecoration(
      color: _surface,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: _rule),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, color: _brand, size: 28),
        const SizedBox(height: 18),
        Text(
          title,
          style: Theme.of(context).textTheme.titleLarge
              ?.copyWith(color: _ink, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 6),
        Text(description, style: const TextStyle(color: _inkSoft)),
        if (busy) ...[
          const SizedBox(height: 18),
          const SizedBox.square(
            dimension: 22,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        ],
        if (action != null) ...[const SizedBox(height: 18), action!],
      ],
    ),
  );
}

class _MessagePanel extends StatelessWidget {
  const _MessagePanel({required this.message, required this.isError});

  final String message;
  final bool isError;

  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: isError ? const Color(0xff3a1619) : const Color(0xff12301f),
      borderRadius: BorderRadius.circular(12),
    ),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(
          isError ? Icons.info_outline : Icons.check_circle_outline,
          color: isError ? const Color(0xffff8a8f) : const Color(0xff5fd39a),
          size: 20,
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            message,
            style: const TextStyle(color: _ink, fontWeight: FontWeight.w600),
          ),
        ),
      ],
    ),
  );
}

class _InviteSheet extends StatefulWidget {
  const _InviteSheet({required this.session, required this.onJoined});

  final SessionController session;
  final VoidCallback onJoined;

  @override
  State<_InviteSheet> createState() => _InviteSheetState();
}

class _InviteSheetState extends State<_InviteSheet> {
  final invite = TextEditingController();
  bool busy = false;
  String? error;

  @override
  void dispose() {
    invite.dispose();
    super.dispose();
  }

  Future<void> join() async {
    if (busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await widget.session.acceptInvite(invite.text);
      if (!mounted) return;
      Navigator.of(context).pop();
      widget.onJoined();
    } catch (failure) {
      if (mounted) setState(() => error = '$failure');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => SafeArea(
    top: false,
    child: AnimatedPadding(
      duration: const Duration(milliseconds: 180),
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 22, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    'Join a league',
                    style: Theme.of(context).textTheme.headlineSmall
                        ?.copyWith(color: _ink, fontWeight: FontWeight.w800),
                  ),
                ),
                IconButton(
                  tooltip: 'Close',
                  onPressed: busy ? null : () => Navigator.of(context).pop(),
                  icon: const Icon(Icons.close),
                ),
              ],
            ),
            const SizedBox(height: 6),
            const Text(
              'Paste the link a commissioner sent you.',
              style: TextStyle(color: _inkSoft),
            ),
            const SizedBox(height: 22),
            TextField(
              key: const Key('invite-link-field'),
              controller: invite,
              keyboardType: TextInputType.url,
              textInputAction: TextInputAction.done,
              autocorrect: false,
              decoration: const InputDecoration(
                labelText: 'Invite link',
                hintText: 'https://…/join/…',
                border: OutlineInputBorder(),
              ),
              onSubmitted: (_) => join(),
            ),
            if (error != null) ...[
              const SizedBox(height: 12),
              Text(error!, style: const TextStyle(color: Color(0xffff8a8f))),
            ],
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              height: 50,
              child: FilledButton(
                onPressed: busy ? null : join,
                child: busy
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Text('Join league'),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}
