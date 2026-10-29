// Presents the website rule book alongside live league rules or public recommended defaults.

import 'package:flutter/material.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import 'rules_content.dart';
import 'rules_section.dart';

Json _map(Object? value) => value is Json ? value : {};
String _label(Object? value) => '${value ?? 'Not set'}'.replaceAll('_', ' ');

class RulesScreen extends StatefulWidget {
  const RulesScreen({super.key, required this.api, this.selectedLeague});
  final ApiClient api;
  final MyLeague? selectedLeague;

  @override
  State<RulesScreen> createState() => _RulesScreenState();
}

class _RulesScreenState extends State<RulesScreen> {
  late Future<_RulesData> future = load();
  String tab = 'general';

  Future<_RulesData> load() async {
    final catalog = await widget.api.competitions();
    final selected = widget.selectedLeague;
    if (selected == null || !selected.ready) return _RulesData(catalog, null);
    final dynasty = await widget.api.dynasty();
    if (dynasty.id != selected.id) {
      throw const ApiException(
        'The selected league is not available through this gameplay API.',
      );
    }
    return _RulesData(catalog, dynasty);
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

  @override
  Widget build(BuildContext context) => FutureBuilder<_RulesData>(
    future: future,
    builder: (context, snapshot) {
      if (!snapshot.hasData &&
          snapshot.connectionState != ConnectionState.done) {
        return const Center(child: CircularProgressIndicator());
      }
      if (snapshot.hasError) {
        return Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text('${snapshot.error}', textAlign: TextAlign.center),
                const SizedBox(height: 12),
                FilledButton.tonal(
                  onPressed: refresh,
                  child: const Text('Retry rules'),
                ),
              ],
            ),
          ),
        );
      }
      final data = snapshot.data!;
      final guides = <_Guide>[];
      if (data.dynasty != null) {
        for (final league in data.dynasty!.leagues) {
          final sport = data.catalog
              .where((item) => item['key'] == league.competition)
              .firstOrNull;
          if (sport != null) {
            guides.add(
              _Guide(league.competition, league.name, league.settings, sport),
            );
          }
        }
      } else {
        for (final sport in data.catalog) {
          guides.add(
            _Guide(
              sport['key'] as String,
              sport['name'] as String,
              _map(sport['defaults']),
              sport,
            ),
          );
        }
      }
      final active =
          tab != 'general' &&
              tab != 'balance' &&
              !guides.any((guide) => guide.key == tab)
          ? 'general'
          : tab;
      final selectedGuide = guides
          .where((guide) => guide.key == active)
          .firstOrNull;
      return RefreshIndicator(
        onRefresh: refresh,
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 36),
          children: [
            Center(
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 900),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Rules',
                      style: Theme.of(context).textTheme.headlineMedium
                          ?.copyWith(fontWeight: FontWeight.w800),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      data.dynasty == null
                          ? 'General rules and recommended sport settings.'
                          : 'General rules and the current settings in ${data.dynasty!.name}.',
                    ),
                    const SizedBox(height: 18),
                    SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: Row(
                        children: [
                          _chip('general', 'Everywhere'),
                          for (final guide in guides)
                            _chip(
                              guide.key,
                              guide.name,
                              color: SportMark.colors[guide.key],
                            ),
                          _chip('balance', 'How it was balanced'),
                        ],
                      ),
                    ),
                    const SizedBox(height: 24),
                    if (active == 'general') _general(context, data.dynasty),
                    if (selectedGuide != null)
                      _sport(context, selectedGuide, data.dynasty != null),
                    if (active == 'balance') _balance(context, data.catalog),
                  ],
                ),
              ),
            ),
          ],
        ),
      );
    },
  );

  Widget _chip(String key, String title, {Color? color}) => Padding(
    padding: const EdgeInsets.only(right: 8),
    child: ChoiceChip(
      selected: tab == key,
      label: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (color != null) ...[
            CircleAvatar(radius: 4, backgroundColor: color),
            const SizedBox(width: 7),
          ],
          Text(title),
        ],
      ),
      onSelected: (_) => setState(() => tab = key),
    ),
  );

  Widget _general(BuildContext context, Dynasty? dynasty) {
    final overall = _map(dynasty?.settings['overall_title']);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Text(
          'These rules apply everywhere. Each sport’s current values are in its own section.',
        ),
        const SizedBox(height: 14),
        for (final topic in generalRules) ...[
          Card(
            margin: const EdgeInsets.only(bottom: 9),
            child: ExpansionTile(
              title: Text(
                topic.title,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
              children: [
                for (final item in topic.items)
                  Padding(
                    padding: const EdgeInsets.only(top: 14),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(
                          item.lead,
                          style: const TextStyle(fontWeight: FontWeight.w700),
                        ),
                        const SizedBox(height: 4),
                        Text(item.detail),
                      ],
                    ),
                  ),
                if (topic.title == 'The basics' && overall['enabled'] == true)
                  Padding(
                    padding: const EdgeInsets.only(top: 14),
                    child: Text(
                      'The overall title adds points from each sport: ${(_map(overall)['points_by_finish'] as List<dynamic>? ?? []).join(', ')} points by finish.',
                    ),
                  ),
              ],
            ),
          ),
        ],
      ],
    );
  }

  Widget _sport(BuildContext context, _Guide guide, bool current) {
    final rules = guide.rules;
    final roster = _map(rules['roster']);
    final lineup = _map(rules['lineup']);
    final scoring = _map(rules['scoring']);
    final format = _map(rules['format']);
    final waivers = _map(rules['waivers']);
    final freeAgency = _map(rules['free_agency']);
    final draft = _map(rules['draft']);
    final trades = _map(rules['trades']);
    final continuity = _map(rules['continuity']);
    final slots = (lineup['slots'] as List<dynamic>? ?? []).map(_map).toList();
    final starters = slots.fold<int>(
      0,
      (count, slot) => count + ((slot['count'] as num?)?.toInt() ?? 0),
    );
    final stats = (guide.catalog['stats'] as List<dynamic>? ?? [])
        .map(_map)
        .toList();
    final namedStats = [
      for (final stat in stats)
        if ((scoring[stat['key']] as num? ?? 0) != 0)
          MapEntry('${stat['label']}', '${scoring[stat['key']]}'),
    ];
    final sections = <Widget>[
      _block(context, 'Roster', [
        _row(
          'Main roster',
          '${roster['main'] ?? 0} players · $starters starters',
        ),
        _row(
          'Reserve',
          '${roster['reserve'] ?? 0} players · ${_label(roster['reserve_eligibility'])}',
        ),
        _row(
          'Reserve lock',
          roster['reserve_lock_season'] == true
              ? 'Until season ends'
              : '${roster['reserve_lock_days'] ?? 0} days',
        ),
      ]),
      _block(context, 'Lineup', [
        for (final slot in slots)
          _row(
            '${slot['count']} × ${slot['name']}',
            (slot['positions'] as List<dynamic>? ?? []).join(', '),
          ),
        _row(
          'Period',
          '${_label(lineup['period'])}${lineup['period'] == 'week' ? ' · starts ${_label(lineup['week_start'])}' : ''}',
        ),
        _row('Locks', _label(lineup['lock'])),
        if ((lineup['pitcher_starts_per_week'] as num? ?? 0) > 0)
          _row('Pitcher starts/week', '${lineup['pitcher_starts_per_week']}'),
        if ((lineup['conferences'] as List<dynamic>? ?? []).isNotEmpty)
          _row(
            'Starting conferences',
            (lineup['conferences'] as List<dynamic>).join(', '),
          ),
      ]),
      _block(
        context,
        'Scoring',
        namedStats.isEmpty
            ? [const Text('No scoring values configured.')]
            : [for (final stat in namedStats) _row(stat.key, stat.value)],
      ),
      _block(context, 'Season', [
        _row('Format', _label(format['type'])),
        if (format['type'] == 'head_to_head') ...[
          _row('Matchup', '${format['matchup_days'] ?? 7} days'),
          _row('Playoff teams', '${format['playoff_teams'] ?? 0}'),
          _row(
            'Bracket',
            format['reseed'] == true
                ? 'Reseeds each round'
                : 'Fixed · no reseeding',
          ),
          _row(
            'Consolation',
            format['consolation'] == false
                ? 'Off'
                : 'On · ${_label(format['consolation_seeding'])}',
          ),
        ],
        if (continuity.isNotEmpty)
          _row(
            'Graduation',
            'Into ${_label(continuity['into'])} · ${_label(continuity['land_on'])}',
          ),
      ]),
      _block(context, 'Free agents and waivers', [
        _row('Free agency', _label(freeAgency['mode'])),
        _row(
          'Weekly adds',
          (freeAgency['weekly_limit'] as num? ?? 0) == 0
              ? 'No limit'
              : '${freeAgency['weekly_limit']}',
        ),
        _row(
          'New entrants',
          freeAgency['new_entrants_draft_only'] == true
              ? 'Wait for rookie draft'
              : 'Available immediately',
        ),
        _row('Waiver method', _label(waivers['mode'])),
        if (waivers['mode'] != 'none')
          _row('Waiver period', '${waivers['hours'] ?? 0} hours'),
        if (waivers['mode'] == 'faab')
          _row('Season budget', '${waivers['budget'] ?? 0}'),
      ]),
      _block(context, 'Rookie draft', [
        _row(
          'Rounds',
          (draft['rounds'] as num? ?? 0) == 0
              ? 'Half the reserve list, rounded up'
              : '${draft['rounds']}',
        ),
        _row('Order', _label(draft['order'])),
        _row(
          'Pick clock',
          (draft['pick_clock_seconds'] as num? ?? 0) == 0
              ? 'None'
              : '${draft['pick_clock_seconds']} seconds',
        ),
        _row('Signing window', '${draft['signing_days'] ?? 0} days'),
        _row('Future picks', '${draft['future_years'] ?? 0} years'),
      ]),
      _block(context, 'Trades', [
        _row('Approval', _label(trades['approval'])),
        _row(
          'Deadline',
          trades['deadline'] == null || trades['deadline'] == ''
              ? 'None'
              : '${trades['deadline']}',
        ),
      ]),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            SportMark(sport: guide.key),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                guide.name,
                style: Theme.of(context).textTheme.titleLarge,
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          current
              ? 'Current league settings. Pull down to refresh after commissioner changes.'
              : 'Recommended starting settings; no playable league is selected.',
        ),
        const SizedBox(height: 18),
        LayoutBuilder(
          builder: (context, constraints) {
            final wide = constraints.maxWidth >= 700;
            return Wrap(
              spacing: 14,
              runSpacing: 14,
              children: [
                for (final section in sections)
                  SizedBox(
                    width: wide
                        ? (constraints.maxWidth - 14) / 2
                        : constraints.maxWidth,
                    child: section,
                  ),
              ],
            );
          },
        ),
      ],
    );
  }

  Widget _block(BuildContext context, String title, List<Widget> children) =>
      DecoratedBox(
        decoration: BoxDecoration(
          color: const Color(0xff141822),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Padding(
          padding: const EdgeInsets.all(17),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                title,
                style: Theme.of(context).textTheme.titleMedium
                    ?.copyWith(fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 9),
              ...children,
            ],
          ),
        ),
      );

  Widget _row(String label, String value) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 6),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Text(label, style: const TextStyle(color: Color(0xffaab2c4))),
        ),
        const SizedBox(width: 14),
        Flexible(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: const TextStyle(fontWeight: FontWeight.w600),
          ),
        ),
      ],
    ),
  );

  Widget _balance(BuildContext context, List<Json> catalog) {
    const values = <String, List<double>>{
      'nba': [29.2, 18.9, 11.3],
      'nfl': [21.3, 15.7, 8.3],
      'cbb': [18.9, 13.8, 7.0],
      'nhl': [16.5, 11.6, 7.4],
      'mlb': [12.0, 8.0, 5.3],
      'wnba': [41.3, 30.0, 15.2],
    };
    String sportName(String key) =>
        '${catalog.where((item) => item['key'] == key).firstOrNull?['name'] ?? key.toUpperCase()}';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'How it was balanced',
          style: Theme.of(context).textTheme.titleLarge,
        ),
        const SizedBox(height: 12),
        const Text(
          'The aim is that a star is worth about the same in every sport, so cross-sport trades can be fair. Recommended rules were tested against last season’s games in simulated 16-team leagues. Each player was measured against the best player left on waivers.',
        ),
        const SizedBox(height: 20),
        _block(context, 'Win probability added per matchup', [
          for (final entry in values.entries)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 7),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      CircleAvatar(
                        radius: 4,
                        backgroundColor: SportMark.colors[entry.key],
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          sportName(entry.key),
                          style: const TextStyle(fontWeight: FontWeight.w700),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 3),
                  Text(
                    '+${entry.value[0].toStringAsFixed(1)}  /  +${entry.value[1].toStringAsFixed(1)}  /  +${entry.value[2].toStringAsFixed(1)}',
                    style: const TextStyle(
                      fontFeatures: [FontFeature.tabularFigures()],
                    ),
                    softWrap: true,
                  ),
                ],
              ),
            ),
          const Padding(
            padding: EdgeInsets.only(top: 8),
            child: Text(
              'Best player / first round / third round. +16 turns a .500 team into a .660 team. Changed league rules will differ.',
            ),
          ),
        ]),
        const SizedBox(height: 20),
        Text(
          'What that led to',
          style: Theme.of(context).textTheme.titleMedium,
        ),
        const SizedBox(height: 10),
        const Text(
          'Basketball counts one game per player each week. Hockey rewards shots and gives goalies meaningful value. Baseball has a utility slot and a pitcher-start cap. Football is full-point PPR with a superflex. WNBA rosters stay short so waivers remain useful.',
        ),
        const SizedBox(height: 20),
        Text(
          'What is still uneven',
          style: Theme.of(context).textTheme.titleMedium,
        ),
        const SizedBox(height: 10),
        const Text(
          'Baseball is the noisiest sport week to week, so its stars move a matchup about half as much as football’s. The WNBA runs the other way: with so few players to go around, its stars are worth close to double. No setting tested at one-week matchups closed either gap.',
        ),
      ],
    );
  }
}

class _RulesData {
  const _RulesData(this.catalog, this.dynasty);
  final List<Json> catalog;
  final Dynasty? dynasty;
}

class _Guide {
  const _Guide(this.key, this.name, this.rules, this.catalog);
  final String key;
  final String name;
  final Json rules;
  final Json catalog;
}
