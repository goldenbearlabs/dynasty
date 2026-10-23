// Renders the shared rule book and live, per-sport settings under My Leagues.

import 'package:flutter/material.dart';

import '../data/models.dart';
import 'rules_content.dart';

Json _map(Object? value) => value is Json ? value : {};
String _label(Object? value) => '${value ?? 'Not set'}'.replaceAll('_', ' ');
String _yesNo(Object? value) => value == true ? 'Yes' : 'No';

class RulesSection extends StatelessWidget {
  const RulesSection({super.key, required this.dynasty});
  final Dynasty dynasty;

  @override
  Widget build(BuildContext context) {
    final overall = _map(dynasty.settings['overall_title']);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Rules', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 4),
        Text(
          'General rules and each sport’s current settings.',
          style: Theme.of(context).textTheme.bodyMedium,
        ),
        const SizedBox(height: 14),
        Card(
          child: ExpansionTile(
            leading: const Icon(Icons.menu_book_outlined),
            title: const Text('Rules for every league'),
            subtitle: Text(
              overall['enabled'] == true
                  ? 'Overall title: ${(_map(overall)['points_by_finish'] as List<dynamic>? ?? []).join(' · ')} points by finish'
                  : 'General dynasty rules',
            ),
            children: [
              for (final topic in generalRules)
                ExpansionTile(
                  title: Text(topic.title),
                  children: [
                    for (final item in topic.items)
                      ListTile(
                        title: Text(item.lead),
                        subtitle: Text(item.detail),
                      ),
                  ],
                ),
            ],
          ),
        ),
        const SizedBox(height: 8),
        for (final league in dynasty.leagues)
          Card(
            child: ExpansionTile(
              leading: SportMark(sport: league.competition),
              title: Text(league.name),
              subtitle: Text('${league.competition.toUpperCase()} rules'),
              children: _sportRules(context, league),
            ),
          ),
      ],
    );
  }

  List<Widget> _sportRules(BuildContext context, League league) {
    final rules = league.settings;
    final format = _map(rules['format']);
    final roster = _map(rules['roster']);
    final lineup = _map(rules['lineup']);
    final freeAgency = _map(rules['free_agency']);
    final waivers = _map(rules['waivers']);
    final draft = _map(rules['draft']);
    final trades = _map(rules['trades']);
    final scoring = _map(rules['scoring']);
    final continuity = _map(rules['continuity']);
    final slots = lineup['slots'] as List<dynamic>? ?? [];

    return [
      _heading(context, 'Format'),
      _value('Competition', _label(format['type'])),
      if (format['type'] == 'head_to_head') ...[
        _value('Matchup days', _label(format['matchup_days'])),
        _value('Playoff teams', _label(format['playoff_teams'])),
      ],
      _heading(context, 'Rosters'),
      _value('Main roster', _label(roster['main'])),
      _value('Reserve roster', _label(roster['reserve'])),
      _value('Reserve eligibility', _label(roster['reserve_eligibility'])),
      _value('Reserve lock', '${roster['reserve_lock_days'] ?? 0} days'),
      _value('Season reserve lock', _yesNo(roster['reserve_lock_season'])),
      _heading(context, 'Lineups'),
      _value('Period', _label(lineup['period'])),
      _value('Week starts', _label(lineup['week_start'])),
      _value('Locks at', _label(lineup['lock'])),
      for (final raw in slots)
        _value(
          'Slot: ${_map(raw)['name']}',
          '${_map(raw)['count'] ?? 0} · ${(_map(raw)['positions'] as List<dynamic>? ?? []).join(', ')}'
              '${(_map(raw)['games_per_week'] as num? ?? 0) > 0 ? ' · ${_map(raw)['games_per_week']} games/week' : ''}',
        ),
      if ((lineup['pitcher_starts_per_week'] as num? ?? 0) > 0)
        _value(
          'Pitcher starts/week',
          _label(lineup['pitcher_starts_per_week']),
        ),
      if ((lineup['conferences'] as List<dynamic>? ?? []).isNotEmpty)
        _value(
          'Starting conferences',
          (lineup['conferences'] as List<dynamic>).join(', '),
        ),
      _heading(context, 'Scoring'),
      if (scoring.isEmpty) _value('Scoring values', 'Not configured'),
      for (final entry in scoring.entries.where((entry) => entry.value != 0))
        _value(_label(entry.key), _label(entry.value)),
      _heading(context, 'Free agency and waivers'),
      _value('Free agency', _label(freeAgency['mode'])),
      _value(
        'New entrants draft only',
        _yesNo(freeAgency['new_entrants_draft_only']),
      ),
      _value(
        'Weekly add limit',
        (freeAgency['weekly_limit'] as num? ?? 0) == 0
            ? 'No limit'
            : _label(freeAgency['weekly_limit']),
      ),
      _value('Waiver method', _label(waivers['mode'])),
      _value('Waiver period', '${waivers['hours'] ?? 0} hours'),
      if (waivers['mode'] == 'faab')
        _value('FAAB budget', _label(waivers['budget'])),
      _heading(context, 'Drafts and trades'),
      _value(
        'Rookie rounds',
        (draft['rounds'] as num? ?? 0) == 0
            ? 'Half the reserve size, rounded up'
            : _label(draft['rounds']),
      ),
      _value('Draft order', _label(draft['order'])),
      _value('Pick clock', '${draft['pick_clock_seconds'] ?? 0} seconds'),
      _value('Future pick years', _label(draft['future_years'])),
      _value('Signing window', '${draft['signing_days'] ?? 0} days'),
      _value('Trade approval', _label(trades['approval'])),
      _value('Trade deadline', _label(trades['deadline'])),
      if (continuity.isNotEmpty)
        _value(
          'Player graduation',
          'Into ${_label(continuity['into'])}, on ${_label(continuity['land_on'])}',
        ),
      const SizedBox(height: 12),
    ];
  }

  Widget _heading(BuildContext context, String title) => Padding(
    padding: const EdgeInsets.fromLTRB(18, 18, 18, 4),
    child: Align(
      alignment: Alignment.centerLeft,
      child: Text(title, style: Theme.of(context).textTheme.titleMedium),
    ),
  );

  Widget _value(String title, String value) => ListTile(
    dense: true,
    title: Text(title),
    trailing: ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 185),
      child: Text(value, textAlign: TextAlign.right, softWrap: true),
    ),
  );
}

class SportMark extends StatelessWidget {
  const SportMark({super.key, required this.sport});
  final String sport;

  static const colors = <String, Color>{
    'cbb': Color(0xffff9547),
    'nba': Color(0xffa48bff),
    'wnba': Color(0xffff8fb3),
    'nhl': Color(0xff4fd1e0),
    'nfl': Color(0xff62d27a),
    'mlb': Color(0xffff7676),
  };

  @override
  Widget build(BuildContext context) => CircleAvatar(
    radius: 20,
    backgroundColor: (colors[sport] ?? Theme.of(context).colorScheme.primary)
        .withValues(alpha: 0.15),
    child: Text(
      sport.toUpperCase(),
      style: TextStyle(
        color: colors[sport] ?? Theme.of(context).colorScheme.primary,
        fontSize: 10,
        fontWeight: FontWeight.w800,
      ),
    ),
  );
}
