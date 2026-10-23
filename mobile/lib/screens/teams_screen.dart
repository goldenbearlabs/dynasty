// Lists public franchises and lets a visitor inspect each team's rosters.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'load_view.dart';

class TeamsScreen extends StatefulWidget {
  const TeamsScreen({super.key, required this.api});
  final ApiClient api;

  @override
  State<TeamsScreen> createState() => _TeamsScreenState();
}

class _TeamsScreenState extends State<TeamsScreen> {
  late Future<Dynasty> future = widget.api.dynasty();
  void refresh() => setState(() => future = widget.api.dynasty());

  @override
  Widget build(BuildContext context) => LoadView(
    future: future,
    onRetry: refresh,
    builder: (dynasty) => ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Text('Franchises', style: Theme.of(context).textTheme.headlineMedium),
        const SizedBox(height: 12),
        if (dynasty.franchises.isEmpty)
          const Text('No franchises have been added yet.'),
        for (final team in dynasty.franchises)
          Card(
            child: ListTile(
              leading: CircleAvatar(
                child: Text(
                  team.name.isEmpty ? '?' : team.name[0].toUpperCase(),
                ),
              ),
              title: Text(team.name),
              subtitle: Text(team.managerName),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) =>
                      TeamDetailScreen(api: widget.api, slug: team.slug),
                ),
              ),
            ),
          ),
      ],
    ),
  );
}

class TeamDetailScreen extends StatefulWidget {
  const TeamDetailScreen({
    super.key,
    required this.api,
    required this.slug,
    this.leagueId,
  });
  final ApiClient api;
  final String slug;
  final String? leagueId;

  @override
  State<TeamDetailScreen> createState() => _TeamDetailScreenState();
}

class _TeamDetailScreenState extends State<TeamDetailScreen> {
  late Future<FranchiseDetail> future = widget.api.franchise(widget.slug);
  void refresh() => setState(() => future = widget.api.franchise(widget.slug));

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Team')),
    body: LoadView(
      future: future,
      onRetry: refresh,
      builder: (detail) => ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Text(
            detail.franchise.name,
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          Text('Manager: ${detail.franchise.managerName}'),
          const SizedBox(height: 16),
          for (final roster in detail.rosters.where(
            (roster) =>
                widget.leagueId == null || roster.leagueId == widget.leagueId,
          )) ...[
            Text(roster.name, style: Theme.of(context).textTheme.titleLarge),
            if (roster.players.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: Text('No players on this roster.'),
              ),
            for (final player in roster.players)
              Card(
                child: ListTile(
                  title: Text(player.name),
                  subtitle: Text(player.positions.join(' · ')),
                  trailing: Text(player.list),
                ),
              ),
            const SizedBox(height: 16),
          ],
        ],
      ),
    ),
  );
}
