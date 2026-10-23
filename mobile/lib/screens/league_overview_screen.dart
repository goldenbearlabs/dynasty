// Opens the dynasty button into an overview of its sports and franchises.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'rules_section.dart';
import 'teams_screen.dart';

class LeagueOverviewScreen extends StatelessWidget {
  const LeagueOverviewScreen({
    super.key,
    required this.dynasty,
    required this.api,
  });
  final Dynasty dynasty;
  final ApiClient api;

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('League overview')),
    body: Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 900),
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            Text(
              dynasty.name,
              style: Theme.of(context).textTheme.headlineMedium
                  ?.copyWith(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            Text(
              '${dynasty.leagues.length} sports · ${dynasty.franchises.length} franchises',
              style: const TextStyle(color: Color(0xffa0a7ba)),
            ),
            const SizedBox(height: 28),
            Text(
              'Sport leagues',
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 8),
            for (final league in dynasty.leagues)
              Card(
                child: ListTile(
                  leading: SportMark(sport: league.competition),
                  title: Text(league.name),
                  subtitle: Text(league.competition.toUpperCase()),
                ),
              ),
            const SizedBox(height: 22),
            Text('Franchises', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            for (final team in dynasty.franchises)
              Card(
                child: ListTile(
                  title: Text(team.name),
                  subtitle: Text(team.managerName),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) =>
                          TeamDetailScreen(api: api, slug: team.slug),
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    ),
  );
}
