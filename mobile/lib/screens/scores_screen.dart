// Reads the public scoreboard for a selected sport and day.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'load_view.dart';

class ScoresScreen extends StatefulWidget {
  const ScoresScreen({super.key, required this.api});
  final ApiClient api;

  @override
  State<ScoresScreen> createState() => _ScoresScreenState();
}

class _ScoresScreenState extends State<ScoresScreen> {
  String sport = 'nhl';
  DateTime day = DateTime.now();
  late Future<Scoreboard> future = load();

  String get dayString =>
      '${day.year.toString().padLeft(4, '0')}-${day.month.toString().padLeft(2, '0')}-${day.day.toString().padLeft(2, '0')}';
  Future<Scoreboard> load() => widget.api.scores(sport, day: dayString);
  void refresh() => setState(() => future = load());

  @override
  Widget build(BuildContext context) => Column(
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 900),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Scores', style: Theme.of(context).textTheme.headlineMedium),
              const SizedBox(height: 14),
              Row(
                children: [
                  Expanded(
                    child: DropdownButtonFormField<String>(
                      initialValue: sport,
                      decoration: const InputDecoration(
                        labelText: 'Sport',
                        border: OutlineInputBorder(),
                      ),
                      items: const [
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
                        sport = value ?? sport;
                        refresh();
                      },
                    ),
                  ),
                  const SizedBox(width: 8),
                  IconButton(
                    tooltip: 'Choose day',
                    icon: const Icon(Icons.calendar_month_outlined),
                    onPressed: () async {
                      final chosen = await showDatePicker(
                        context: context,
                        initialDate: day,
                        firstDate: DateTime(2020),
                        lastDate: DateTime(2100),
                      );
                      if (chosen != null) {
                        day = chosen;
                        refresh();
                      }
                    },
                  ),
                  IconButton(
                    tooltip: 'Refresh scores',
                    onPressed: refresh,
                    icon: const Icon(Icons.refresh),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
      Expanded(
        child: LoadView(
          future: future,
          onRetry: refresh,
          builder: (board) => ListView(
            padding: const EdgeInsets.all(20),
            children: [
              Text(
                '${sport.toUpperCase()} · ${board.day}',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              if (board.games.isEmpty)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 20),
                  child: Text('No games on this day.'),
                ),
              for (final game in board.games)
                Card(
                  child: ListTile(
                    title: Text(
                      '${game.away}  ${game.awayScore}  —  ${game.homeScore}  ${game.home}',
                    ),
                    subtitle: Text(
                      game.detail.isEmpty ? game.status : game.detail,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    ],
  );
}
