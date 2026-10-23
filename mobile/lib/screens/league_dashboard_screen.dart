// Presents the current playable league's standings and activity from the same API as the website.
// Pull-to-refresh updates the snapshot; unavailable playoff odds do not fail the page.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'load_view.dart';

class LeagueDashboardScreen extends StatefulWidget {
  const LeagueDashboardScreen({
    super.key,
    required this.api,
    required this.dynasty,
    required this.league,
  });

  final ApiClient api;
  final Dynasty dynasty;
  final MyLeague league;

  @override
  State<LeagueDashboardScreen> createState() => _LeagueDashboardScreenState();
}

class _LeagueDashboardScreenState extends State<LeagueDashboardScreen> {
  late Future<_DashboardData> future = load();
  String standing = 'overall';
  String sport = 'all';
  String activityType = 'all';

  Future<LeagueOdds?> optionalOdds() async {
    try {
      return await widget.api.odds().timeout(const Duration(seconds: 3));
    } catch (_) {
      return null;
    }
  }

  Future<_DashboardData> load() async {
    final standings = widget.api.standings();
    final overall = widget.api.overall();
    final activity = widget.api.activity();
    final odds = optionalOdds();
    return _DashboardData(
      standings: await standings,
      overall: await overall,
      activity: await activity,
      odds: await odds,
    );
  }

  Future<void> refresh() async {
    final next = load();
    setState(() {
      future = next;
    });
    try {
      await next;
    } catch (_) {}
  }

  String franchiseName(String id, {String? leagueId}) {
    if (leagueId != null) {
      for (final identity in widget.dynasty.teamIdentities) {
        if (identity.franchiseId == id &&
            identity.leagueId == leagueId &&
            identity.name.isNotEmpty) {
          return identity.name;
        }
      }
    }
    for (final franchise in widget.dynasty.franchises) {
      if (franchise.id == id) return franchise.name;
    }
    return 'Franchise';
  }

  String points(double value) => value == value.roundToDouble()
      ? value.toStringAsFixed(0)
      : value.toStringAsFixed(1);

  Widget standingsSection(BuildContext context, _DashboardData data) {
    final latest = data.overall.isEmpty ? null : data.overall.first;
    final tabs = [
      if (latest != null) 'overall',
      ...data.standings.map((table) => table.competition),
    ];
    final current = tabs.contains(standing)
        ? standing
        : (tabs.isEmpty ? '' : tabs.first);
    StandingsTable? table;
    for (final value in data.standings) {
      if (value.competition == current) table = value;
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Standings', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 12),
        if (tabs.isEmpty)
          const Card(
            child: Padding(
              padding: EdgeInsets.all(20),
              child: Text('No standings yet.'),
            ),
          )
        else ...[
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: [
                for (final tab in tabs) ...[
                  ChoiceChip(
                    label: Text(
                      tab == 'overall' ? 'Overall' : tab.toUpperCase(),
                    ),
                    selected: current == tab,
                    onSelected: (_) => setState(() {
                      standing = tab;
                      sport = tab == 'overall' ? 'all' : tab;
                    }),
                  ),
                  const SizedBox(width: 8),
                ],
              ],
            ),
          ),
          const SizedBox(height: 12),
          if (current == 'overall' && latest != null) ...[
            Text('${latest.year} overall${latest.finalYear ? ' · Final' : ''}'),
            const SizedBox(height: 8),
            if (latest.rows.isEmpty) const Text('No overall points yet.'),
            for (var i = 0; i < latest.rows.length; i++)
              Card(
                child: ListTile(
                  leading: CircleAvatar(child: Text('${i + 1}')),
                  title: Text(franchiseName(latest.rows[i].franchiseId)),
                  subtitle: Text(
                    'Adjusted: ${points(latest.rows[i].adjustedPoints)}',
                  ),
                  trailing: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    crossAxisAlignment: CrossAxisAlignment.end,
                    children: [
                      Text('${points(latest.rows[i].points)} pts'),
                      if (!latest.finalYear && data.odds != null)
                        Text(
                          _titleChance(data.odds!, latest.rows[i].franchiseId),
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                    ],
                  ),
                ),
              ),
          ] else if (table != null) ...[
            if (table.season == null)
              const Card(
                child: Padding(
                  padding: EdgeInsets.all(20),
                  child: Text(
                    'No season yet. The commissioner starts each sport season.',
                  ),
                ),
              )
            else if (table.rows.isEmpty)
              const Text('No standings yet.')
            else
              for (var i = 0; i < table.rows.length; i++)
                Card(
                  child: ListTile(
                    leading: CircleAvatar(child: Text('${i + 1}')),
                    title: Text(
                      franchiseName(
                        table.rows[i].franchiseId,
                        leagueId: table.leagueId,
                      ),
                    ),
                    subtitle: Text(
                      table.format == 'head_to_head'
                          ? '${table.rows[i].wins}-${table.rows[i].losses}'
                                '${table.rows[i].ties == 0 ? '' : '-${table.rows[i].ties}'}'
                                '  ·  Against ${points(table.rows[i].pointsAgainst)}'
                          : 'Season points',
                    ),
                    trailing: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      crossAxisAlignment: CrossAxisAlignment.end,
                      children: [
                        Text(
                          '${points(table.format == 'head_to_head' ? table.rows[i].pointsFor : table.rows[i].points)} pts',
                        ),
                        if (table.format == 'head_to_head' &&
                            table.season?['status'] == 'active' &&
                            data.odds != null)
                          Text(
                            _playoffChance(
                              data.odds!,
                              table.competition,
                              table.rows[i].franchiseId,
                            ),
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                      ],
                    ),
                  ),
                ),
          ],
        ],
      ],
    );
  }

  String _titleChance(LeagueOdds odds, String id) {
    for (final row in odds.overall) {
      if (row.franchiseId == id) {
        return '${(row.title * 100).toStringAsFixed(1)}% title';
      }
    }
    return '';
  }

  String _playoffChance(LeagueOdds odds, String competition, String id) {
    for (final league in odds.leagues) {
      if (league.competition != competition) continue;
      for (final row in league.rows) {
        if (row.franchiseId == id) {
          return '${(row.playoffs * 100).toStringAsFixed(1)}% playoffs';
        }
      }
    }
    return '';
  }

  bool matchesType(ActivityItem item) {
    if (activityType == 'all') return true;
    if (activityType == 'roster') {
      return const {
        'add',
        'drop',
        'claim',
        'sign',
        'unsigned',
        'move',
        'graduated',
        'released',
      }.contains(item.kind);
    }
    return item.kind == activityType;
  }

  String describe(ActivityItem item) {
    if (item.kind == 'settings') return 'League rules changed';
    if (item.kind == 'trade') {
      return '${item.franchiseName} acquired ${item.playerName.isEmpty ? 'a draft pick' : item.playerName}';
    }
    const verbs = {
      'add': 'added',
      'drop': 'dropped',
      'claim': 'claimed',
      'pick': 'drafted',
      'sign': 'signed',
      'move': 'moved',
      'graduated': 'kept',
      'released': 'lost',
    };
    return '${item.franchiseName} ${verbs[item.kind] ?? item.kind} ${item.playerName}'
        .trim();
  }

  Widget activitySection(BuildContext context, _DashboardData data) {
    final filtered = data.activity
        .where(
          (item) =>
              (sport == 'all' || item.competition == sport) &&
              matchesType(item),
        )
        .toList();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                'Recent activity',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
            ),
            if (sport != 'all' || activityType != 'all')
              TextButton(
                onPressed: () => setState(() {
                  sport = 'all';
                  activityType = 'all';
                }),
                child: const Text('Clear filters'),
              ),
          ],
        ),
        const SizedBox(height: 12),
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              for (final choice in [
                'all',
                ...widget.dynasty.leagues.map((league) => league.competition),
              ]) ...[
                ChoiceChip(
                  label: Text(
                    choice == 'all' ? 'All sports' : choice.toUpperCase(),
                  ),
                  selected: sport == choice,
                  onSelected: (_) => setState(() => sport = choice),
                ),
                const SizedBox(width: 8),
              ],
            ],
          ),
        ),
        const SizedBox(height: 8),
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              for (final choice in const [
                'all',
                'trade',
                'roster',
                'settings',
              ]) ...[
                ChoiceChip(
                  label: Text(switch (choice) {
                    'all' => 'All activity',
                    'trade' => 'Trades',
                    'roster' => 'Roster moves',
                    _ => 'Rule changes',
                  }),
                  selected: activityType == choice,
                  onSelected: (_) => setState(() => activityType = choice),
                ),
                const SizedBox(width: 8),
              ],
            ],
          ),
        ),
        const SizedBox(height: 12),
        if (filtered.isEmpty)
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Text(
                data.activity.isEmpty
                    ? 'Nothing has happened yet.'
                    : 'No activity matches these filters.',
              ),
            ),
          ),
        for (final item in filtered)
          Card(
            child: ListTile(
              leading: CircleAvatar(
                radius: 18,
                child: Text(
                  item.competition.isEmpty
                      ? 'MF'
                      : item.competition.toUpperCase().substring(0, 2),
                ),
              ),
              title: Text(describe(item)),
              subtitle: Text(
                '${item.competition.toUpperCase()}${item.createdAt == null ? '' : ' · ${item.createdAt!.toLocal().month}/${item.createdAt!.toLocal().day}'}',
              ),
            ),
          ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) => LoadView(
    future: future,
    onRetry: refresh,
    builder: (data) => RefreshIndicator(
      onRefresh: refresh,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(20, 20, 20, 100),
        children: [
          Text(
            'LEAGUE OVERVIEW',
            style: TextStyle(
              color: Theme.of(context).colorScheme.primary,
              fontSize: 12,
              fontWeight: FontWeight.w800,
              letterSpacing: 2,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            widget.dynasty.name,
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          const SizedBox(height: 6),
          Text(
            '${widget.dynasty.leagues.length} sports · ${widget.dynasty.franchises.length} franchises',
          ),
          const SizedBox(height: 28),
          standingsSection(context, data),
          const SizedBox(height: 28),
          activitySection(context, data),
        ],
      ),
    ),
  );
}

class _DashboardData {
  const _DashboardData({
    required this.standings,
    required this.overall,
    required this.activity,
    required this.odds,
  });

  final List<StandingsTable> standings;
  final List<OverallYear> overall;
  final List<ActivityItem> activity;
  final LeagueOdds? odds;
}
