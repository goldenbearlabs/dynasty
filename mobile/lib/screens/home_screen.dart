// Shows one dark My Leagues landing page, the manager's teams, and league rules.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/session_controller.dart';
import 'auth_screen.dart';
import 'league_overview_screen.dart';
import 'load_view.dart';
import 'rules_section.dart';
import 'teams_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.api, required this.session});
  final ApiClient api;
  final SessionController session;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  late Future<Dynasty> dynastyFuture = widget.api.dynasty();
  Future<FranchiseDetail>? teamFuture;
  String? loadedSlug;

  @override
  void initState() {
    super.initState();
    widget.session.addListener(syncSession);
    syncSession();
  }

  @override
  void dispose() {
    widget.session.removeListener(syncSession);
    super.dispose();
  }

  void syncSession() {
    final slug = widget.session.session?.franchise?.slug;
    if (loadedSlug == slug) return;
    loadedSlug = slug;
    if (mounted) {
      setState(() {
        teamFuture = slug == null ? null : widget.api.franchise(slug);
      });
    }
  }

  Future<void> refresh() async {
    setState(() {
      dynastyFuture = widget.api.dynasty();
      final slug = widget.session.session?.franchise?.slug;
      teamFuture = slug == null ? null : widget.api.franchise(slug);
    });
    try {
      await dynastyFuture;
    } catch (_) {}
  }

  void openSignIn() => Navigator.of(context).push(
    MaterialPageRoute<void>(
      builder: (_) => AuthScreen(session: widget.session),
    ),
  );

  Future<void> signOut() async {
    try {
      await widget.session.logout();
    } catch (error) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Could not sign out: $error')));
      }
    }
  }

  @override
  Widget build(BuildContext context) => LoadView(
    future: dynastyFuture,
    onRetry: refresh,
    builder: (dynasty) => RefreshIndicator(
      onRefresh: refresh,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(20, 22, 20, 48),
        children: [
          _brand(context),
          const SizedBox(height: 32),
          Text(
            'MY LEAGUES',
            style: TextStyle(
              color: Theme.of(context).colorScheme.primary,
              fontSize: 12,
              fontWeight: FontWeight.w800,
              letterSpacing: 2,
            ),
          ),
          const SizedBox(height: 12),
          _leagueButton(context, dynasty),
          const SizedBox(height: 26),
          _teams(context),
          const SizedBox(height: 32),
          RulesSection(dynasty: dynasty),
        ],
      ),
    ),
  );

  Widget _brand(BuildContext context) => Row(
    children: [
      Container(
        width: 44,
        height: 44,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: Theme.of(context).colorScheme.primary,
          borderRadius: BorderRadius.circular(13),
        ),
        child: const Text(
          'MF',
          style: TextStyle(
            color: Color(0xff0b0e15),
            fontWeight: FontWeight.w900,
            fontSize: 17,
            letterSpacing: -1,
          ),
        ),
      ),
      const SizedBox(width: 12),
      Expanded(
        child: Text(
          'Mosaic Fantasy',
          style: Theme.of(context).textTheme.titleLarge
              ?.copyWith(fontWeight: FontWeight.w800),
        ),
      ),
      if (widget.session.session != null)
        IconButton(
          tooltip: 'Sign out',
          icon: const Icon(Icons.logout),
          onPressed: signOut,
        ),
    ],
  );

  Widget _leagueButton(BuildContext context, Dynasty dynasty) => Card(
    margin: EdgeInsets.zero,
    child: InkWell(
      borderRadius: BorderRadius.circular(12),
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) =>
              LeagueOverviewScreen(dynasty: dynasty, api: widget.api),
        ),
      ),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(
                  Icons.emoji_events_outlined,
                  color: Color(0xfff0c050),
                ),
                const SizedBox(width: 8),
                Text(
                  widget.session.session == null
                      ? 'DYNASTY LEAGUE'
                      : 'YOUR DYNASTY',
                  style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w800,
                    letterSpacing: 1.6,
                    color: Color(0xffa0a7ba),
                  ),
                ),
                const Spacer(),
                const Icon(Icons.arrow_forward_ios, size: 15),
              ],
            ),
            const SizedBox(height: 14),
            Text(
              dynasty.name == 'Mosaic Fantasy'
                  ? 'League overview'
                  : dynasty.name,
              style: Theme.of(context).textTheme.headlineMedium
                  ?.copyWith(fontWeight: FontWeight.w800, letterSpacing: -0.6),
            ),
            const SizedBox(height: 5),
            Text(
              '${dynasty.leagues.length} sports  ·  ${dynasty.franchises.length} franchises',
              style: const TextStyle(color: Color(0xffa0a7ba)),
            ),
            const SizedBox(height: 14),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final league in dynasty.leagues)
                  SportMark(sport: league.competition),
              ],
            ),
          ],
        ),
      ),
    ),
  );

  Widget _teams(BuildContext context) {
    if (!widget.session.ready) {
      return const Center(child: CircularProgressIndicator());
    }
    if (widget.session.session == null) {
      return Card(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Your teams', style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 8),
              const Text('Sign in to see the teams you manage in each sport.'),
              if (widget.session.restoreError != null) ...[
                const SizedBox(height: 8),
                Text(
                  widget.session.restoreError!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
                TextButton(
                  onPressed: widget.session.restore,
                  child: const Text('Retry session'),
                ),
              ],
              const SizedBox(height: 14),
              FilledButton.icon(
                onPressed: openSignIn,
                icon: const Icon(Icons.login),
                label: const Text('Sign in or join'),
              ),
            ],
          ),
        ),
      );
    }

    final future = teamFuture;
    if (future == null) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('My Franchises', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 5),
        Text(
          widget.session.session!.franchise!.name,
          style: const TextStyle(color: Color(0xffa0a7ba)),
        ),
        const SizedBox(height: 12),
        FutureBuilder<FranchiseDetail>(
          future: future,
          builder: (context, snapshot) {
            if (!snapshot.hasData && !snapshot.hasError) {
              return const Padding(
                padding: EdgeInsets.all(20),
                child: Center(child: CircularProgressIndicator()),
              );
            }
            if (snapshot.hasError) {
              return Card(
                child: ListTile(
                  title: const Text('Could not load your teams'),
                  subtitle: Text('${snapshot.error}'),
                  trailing: IconButton(
                    onPressed: refresh,
                    icon: const Icon(Icons.refresh),
                  ),
                ),
              );
            }
            final rosters = snapshot.data!.rosters;
            if (rosters.isEmpty) {
              return const Text(
                'No sport teams have been added to your franchise yet.',
              );
            }
            return Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                for (final roster in rosters)
                  SizedBox(
                    width: MediaQuery.sizeOf(context).width >= 700
                        ? 270
                        : double.infinity,
                    child: Card(
                      margin: EdgeInsets.zero,
                      child: InkWell(
                        borderRadius: BorderRadius.circular(12),
                        onTap: () => Navigator.of(context).push(
                          MaterialPageRoute<void>(
                            builder: (_) => TeamDetailScreen(
                              api: widget.api,
                              slug: widget.session.session!.franchise!.slug,
                              leagueId: roster.leagueId,
                            ),
                          ),
                        ),
                        child: Padding(
                          padding: const EdgeInsets.all(15),
                          child: Row(
                            children: [
                              SportMark(sport: roster.competition),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      roster.name,
                                      style: const TextStyle(
                                        fontWeight: FontWeight.w700,
                                      ),
                                    ),
                                    Text(
                                      '${roster.players.length} players · ${roster.competition.toUpperCase()}',
                                      style: const TextStyle(
                                        color: Color(0xffa0a7ba),
                                        fontSize: 12,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                              const Icon(Icons.chevron_right),
                            ],
                          ),
                        ),
                      ),
                    ),
                  ),
              ],
            );
          },
        ),
      ],
    );
  }
}
