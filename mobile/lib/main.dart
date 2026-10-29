// Starts Mosaic Fantasy and adapts its main navigation to iPhone and iPad.

import 'package:flutter/material.dart';

import 'config/app_config.dart';
import 'data/api_client.dart';
import 'data/models.dart';
import 'data/session_controller.dart';
import 'screens/auth_screen.dart';
import 'screens/account_screen.dart';
import 'screens/league_dashboard_screen.dart';
import 'screens/leagues_screen.dart';
import 'screens/players_screen.dart';
import 'screens/profile_avatar.dart';
import 'screens/rules_screen.dart';
import 'screens/scores_screen.dart';
import 'screens/teams_screen.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final api = ApiClient(baseUrl: AppConfig.apiBaseUrl);
  final session = SessionController(api: api, store: const SecureTokenStore());
  api.tokenProvider = () => session.token;
  session.restore();
  runApp(MosaicApp(api: api, session: session));
}

class MosaicApp extends StatelessWidget {
  const MosaicApp({super.key, required this.api, required this.session});

  final ApiClient api;
  final SessionController session;

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'Mosaic Fantasy',
    debugShowCheckedModeBanner: false,
    theme: ThemeData(
      brightness: Brightness.dark,
      colorScheme: ColorScheme.fromSeed(
        seedColor: const Color(0xff8499ff),
        brightness: Brightness.dark,
        surface: const Color(0xff141822),
      ),
      scaffoldBackgroundColor: const Color(0xff0b0e15),
      cardColor: const Color(0xff141822),
      dividerColor: const Color(0xff252b3a),
      appBarTheme: const AppBarTheme(backgroundColor: Color(0xff0b0e15)),
      navigationBarTheme: const NavigationBarThemeData(
        backgroundColor: Color(0xff141822),
        indicatorColor: Color(0xff1b2347),
      ),
      navigationRailTheme: const NavigationRailThemeData(
        backgroundColor: Color(0xff141822),
        indicatorColor: Color(0xff1b2347),
      ),
      useMaterial3: true,
    ),
    home: AppShell(api: api, session: session),
  );
}

class AppShell extends StatefulWidget {
  const AppShell({super.key, required this.api, required this.session});
  final ApiClient api;
  final SessionController session;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> with WidgetsBindingObserver {
  MyLeague? selectedLeague;
  Dynasty? selectedDynasty;
  String section = 'leagues';

  @override
  void initState() {
    super.initState();
    widget.api.tokenProvider = () => widget.session.token;
    widget.session.addListener(onSessionChanged);
    WidgetsBinding.instance.addObserver(this);
  }

  void onSessionChanged() {
    if (!mounted) return;
    setState(() {
      if (widget.session.session == null ||
          (selectedLeague != null &&
              widget.session.session?.franchise?.id !=
                  selectedLeague!.franchiseId)) {
        selectedLeague = null;
        selectedDynasty = null;
        section = 'leagues';
      }
    });
  }

  void openLeague(MyLeague league, Dynasty dynasty) => setState(() {
    selectedLeague = league;
    selectedDynasty = dynasty;
    section = 'league';
  });

  void navigate(String destination) {
    Navigator.of(context).pop();
    setState(() => section = destination);
  }

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

  Widget drawerEntry(String label, IconData icon, String? destination) =>
      ListTile(
        leading: Icon(icon),
        title: Text(label),
        trailing: destination == null ? const Text('Coming soon') : null,
        enabled: destination != null,
        selected: destination == section,
        onTap: destination == null ? null : () => navigate(destination),
      );

  Widget appDrawer() => Drawer(
    child: SafeArea(
      child: ListView(
        children: [
          Material(
            color: Colors.transparent,
            child: InkWell(
              onTap: () => navigate('account'),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(18, 22, 18, 20),
                child: Row(
                  children: [
                    ProfileAvatar(
                      name:
                          widget.session.session?.franchise?.name ??
                          widget.session.session?.email ??
                          'Mosaic Fantasy',
                      imageUrl:
                          widget.session.session?.franchise?.imageUrl ?? '',
                      size: 54,
                    ),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Text(
                        widget.session.session?.franchise?.managerName
                                    .trim()
                                    .isNotEmpty ==
                                true
                            ? widget.session.session!.franchise!.managerName
                            : widget.session.session?.email ?? '',
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: Theme.of(context).textTheme.titleLarge
                            ?.copyWith(fontWeight: FontWeight.w700),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          const Divider(),
          drawerEntry('Leagues', Icons.home_outlined, 'leagues'),
          drawerEntry(
            'League',
            Icons.emoji_events_outlined,
            selectedLeague == null ? null : 'league',
          ),
          drawerEntry('My team', Icons.shield_outlined, null),
          drawerEntry('Matchups', Icons.sports_score_outlined, null),
          drawerEntry(
            'Franchises',
            Icons.groups_outlined,
            selectedLeague == null ? null : 'teams',
          ),
          drawerEntry(
            'Players',
            Icons.person_search_outlined,
            selectedLeague == null ? null : 'players',
          ),
          drawerEntry('Watchlist', Icons.star_outline, null),
          drawerEntry('Research', Icons.search, null),
          drawerEntry('Waivers', Icons.schedule_outlined, null),
          drawerEntry('Drafts', Icons.view_list_outlined, null),
          drawerEntry('Trades', Icons.swap_horiz_outlined, null),
          drawerEntry(
            'Scores',
            Icons.scoreboard_outlined,
            selectedLeague == null ? null : 'scores',
          ),
          drawerEntry('Rules', Icons.menu_book_outlined, 'rules'),
          if (selectedLeague != null &&
              widget.session.session?.franchise?.isCommissioner == true)
            drawerEntry('Commissioner', Icons.settings_outlined, null),
        ],
      ),
    ),
  );

  Widget currentScreen() {
    if (section == 'account') {
      return AccountScreen(
        key: ValueKey('account-${selectedLeague?.id ?? 'none'}'),
        api: widget.api,
        session: widget.session,
        selectedLeague: selectedLeague,
        onSignOut: signOut,
      );
    }
    if (section == 'rules') {
      return RulesScreen(
        key: ValueKey('rules-${selectedLeague?.id ?? 'none'}'),
        api: widget.api,
        selectedLeague: selectedLeague,
      );
    }
    if (section == 'leagues' ||
        selectedLeague == null ||
        selectedDynasty == null) {
      return LeaguesScreen(
        key: ValueKey(widget.session.session?.franchise?.id ?? 'no-league'),
        api: widget.api,
        session: widget.session,
        onOpen: openLeague,
      );
    }
    switch (section) {
      case 'league':
        return LeagueDashboardScreen(
          key: ValueKey(selectedLeague!.id),
          api: widget.api,
          dynasty: selectedDynasty!,
          league: selectedLeague!,
        );
      case 'teams':
        return TeamsScreen(api: widget.api);
      case 'players':
        return PlayersScreen(api: widget.api);
      case 'scores':
        return ScoresScreen(api: widget.api);
      default:
        return const Center(child: Text('This section is coming soon.'));
    }
  }

  @override
  void dispose() {
    widget.session.removeListener(onSessionChanged);
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed && widget.session.ready) {
      widget.session.refresh();
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.session.ready) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (widget.session.session == null) {
      return AuthScreen(session: widget.session);
    }
    return Scaffold(
      appBar: section == 'leagues'
          ? null
          : AppBar(
              title: Text(
                section == 'league'
                    ? selectedLeague?.name ?? 'League'
                    : section[0].toUpperCase() + section.substring(1),
              ),
              actions: [
                if (selectedLeague != null)
                  IconButton(
                    tooltip: 'Choose league',
                    icon: const Icon(Icons.swap_horiz),
                    onPressed: () => setState(() => section = 'leagues'),
                  ),
              ],
            ),
      drawer: appDrawer(),
      body: SafeArea(child: currentScreen()),
      bottomNavigationBar:
          selectedLeague == null ||
              section == 'leagues' ||
              section == 'account' ||
              section == 'rules'
          ? null
          : SafeArea(
              top: false,
              child: Center(
                heightFactor: 1,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                  child: Material(
                    color: const Color(0xff141822),
                    borderRadius: BorderRadius.circular(32),
                    clipBehavior: Clip.antiAlias,
                    child: InkWell(
                      onTap: () => setState(() => section = 'league'),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 28,
                          vertical: 11,
                        ),
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              section == 'league'
                                  ? Icons.emoji_events
                                  : Icons.emoji_events_outlined,
                            ),
                            const Text('League'),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
    );
  }
}
