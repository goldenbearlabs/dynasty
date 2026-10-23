// Offers a public player directory with search, sport filtering, and paging.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'load_view.dart';

class PlayersScreen extends StatefulWidget {
  const PlayersScreen({super.key, required this.api});
  final ApiClient api;

  @override
  State<PlayersScreen> createState() => _PlayersScreenState();
}

class _PlayersScreenState extends State<PlayersScreen> {
  final search = TextEditingController();
  String sport = '';
  String query = '';
  int page = 1;
  late Future<PlayerPage> future = load();

  Future<PlayerPage> load() =>
      widget.api.players(competition: sport, query: query, page: page);
  void refresh() => setState(() => future = load());

  @override
  void dispose() {
    search.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Column(
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 900),
          child: Column(
            children: [
              Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  'Players',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
              ),
              const SizedBox(height: 14),
              TextField(
                controller: search,
                decoration: InputDecoration(
                  labelText: 'Search players',
                  prefixIcon: const Icon(Icons.search),
                  suffixIcon: IconButton(
                    icon: const Icon(Icons.search),
                    onPressed: () {
                      query = search.text.trim();
                      page = 1;
                      refresh();
                    },
                  ),
                  border: const OutlineInputBorder(),
                ),
                onSubmitted: (value) {
                  query = value.trim();
                  page = 1;
                  refresh();
                },
              ),
              const SizedBox(height: 8),
              DropdownButtonFormField<String>(
                initialValue: sport,
                decoration: const InputDecoration(
                  labelText: 'Sport',
                  border: OutlineInputBorder(),
                ),
                items: const [
                  DropdownMenuItem(value: '', child: Text('All sports')),
                  DropdownMenuItem(value: 'nfl', child: Text('NFL')),
                  DropdownMenuItem(value: 'nba', child: Text('NBA')),
                  DropdownMenuItem(value: 'wnba', child: Text('WNBA')),
                  DropdownMenuItem(
                    value: 'cbb',
                    child: Text('College Basketball'),
                  ),
                  DropdownMenuItem(value: 'mlb', child: Text('MLB')),
                  DropdownMenuItem(value: 'nhl', child: Text('NHL')),
                ],
                onChanged: (value) {
                  sport = value ?? '';
                  page = 1;
                  refresh();
                },
              ),
            ],
          ),
        ),
      ),
      Expanded(
        child: LoadView(
          future: future,
          onRetry: refresh,
          builder: (result) => ListView(
            padding: const EdgeInsets.all(20),
            children: [
              Text(
                '${result.total} players',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              if (result.players.isEmpty)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 20),
                  child: Text('No players match these filters.'),
                ),
              for (final player in result.players)
                Card(
                  child: ListTile(
                    title: Text(player.name),
                    subtitle: Text(
                      [
                        player.competition.toUpperCase(),
                        player.team,
                        player.positions.join('/'),
                      ].where((part) => part.isNotEmpty).join(' · '),
                    ),
                    trailing: player.owner.isEmpty
                        ? null
                        : const Icon(Icons.shield_outlined),
                  ),
                ),
              if (result.total > result.perPage)
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    IconButton(
                      tooltip: 'Previous page',
                      onPressed: page > 1
                          ? () {
                              page--;
                              refresh();
                            }
                          : null,
                      icon: const Icon(Icons.chevron_left),
                    ),
                    Text('Page ${result.page}'),
                    IconButton(
                      tooltip: 'Next page',
                      onPressed: page * result.perPage < result.total
                          ? () {
                              page++;
                              refresh();
                            }
                          : null,
                      icon: const Icon(Icons.chevron_right),
                    ),
                  ],
                ),
            ],
          ),
        ),
      ),
    ],
  );
}
