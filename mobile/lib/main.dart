// Starts Mosaic Fantasy and adapts its main navigation to iPhone and iPad.

import 'package:flutter/material.dart';

import 'config/app_config.dart';
import 'data/api_client.dart';
import 'data/session_controller.dart';
import 'screens/home_screen.dart';
import 'screens/account_lobby_screen.dart';
import 'screens/auth_screen.dart';
import 'screens/players_screen.dart';
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
  int selected = 0;

  @override
  void initState() {
    super.initState();
    widget.api.tokenProvider = () => widget.session.token;
    widget.session.addListener(onSessionChanged);
    WidgetsBinding.instance.addObserver(this);
  }

  void onSessionChanged() {
    if (mounted) setState(() {});
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
    if (widget.session.session!.franchise == null) {
      return AccountLobbyScreen(session: widget.session);
    }
    final destinations = [
      const NavigationDestination(
        icon: Icon(Icons.home_outlined),
        selectedIcon: Icon(Icons.home),
        label: 'Home',
      ),
      const NavigationDestination(
        icon: Icon(Icons.groups_outlined),
        label: 'Teams',
      ),
      const NavigationDestination(
        icon: Icon(Icons.person_search_outlined),
        label: 'Players',
      ),
      const NavigationDestination(
        icon: Icon(Icons.sports_score_outlined),
        label: 'Scores',
      ),
    ];
    final screens = [
      HomeScreen(api: widget.api, session: widget.session),
      TeamsScreen(api: widget.api),
      PlayersScreen(api: widget.api),
      ScoresScreen(api: widget.api),
    ];
    final wide = MediaQuery.sizeOf(context).width >= 700;

    return Scaffold(
      body: SafeArea(
        top: true,
        bottom: false,
        child: wide
            ? Row(
                children: [
                  NavigationRail(
                    selectedIndex: selected,
                    onDestinationSelected: (value) =>
                        setState(() => selected = value),
                    labelType: NavigationRailLabelType.all,
                    destinations: destinations
                        .map(
                          (item) => NavigationRailDestination(
                            icon: item.icon,
                            selectedIcon: item.selectedIcon,
                            label: Text(item.label),
                          ),
                        )
                        .toList(),
                  ),
                  const VerticalDivider(width: 1),
                  Expanded(child: screens[selected]),
                ],
              )
            : screens[selected],
      ),
      bottomNavigationBar: wide
          ? null
          : NavigationBar(
              selectedIndex: selected,
              onDestinationSelected: (value) =>
                  setState(() => selected = value),
              destinations: destinations,
            ),
    );
  }
}
