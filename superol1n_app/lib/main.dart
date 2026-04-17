import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'core/config.dart';
import 'core/settings_screen.dart';
import 'core/websocket_service.dart';
import 'modules/ai/ai_provider.dart';
import 'modules/ai/ai_screen.dart';
import 'modules/downloads/downloads_provider.dart';
import 'modules/downloads/downloads_screen.dart';
import 'modules/events/events_provider.dart';
import 'modules/events/events_screen.dart';
import 'modules/media/media_provider.dart';
import 'modules/media/media_screen.dart';
import 'shared/theme/app_theme.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await AppConfig.load();
  runApp(const SuperOl1nApp());
}

class SuperOl1nApp extends StatelessWidget {
  const SuperOl1nApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        ChangeNotifierProvider(create: (_) => AiProvider()),
        ChangeNotifierProvider(create: (_) => MediaProvider()),
        ChangeNotifierProvider(create: (_) => DownloadsProvider()),
        ChangeNotifierProvider(
            create: (_) => EventsProvider(WebSocketService())),
      ],
      child: MaterialApp(
        title: 'SuperOl1n',
        theme: appTheme,
        home: const MainShell(),
      ),
    );
  }
}

class MainShell extends StatefulWidget {
  const MainShell({super.key});

  @override
  State<MainShell> createState() => _MainShellState();
}

class _MainShellState extends State<MainShell> {
  int _index = 0;

  static const _screens = [
    AiScreen(),
    MediaScreen(),
    DownloadsScreen(),
    EventsScreen(),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: _index == 0
          ? null
          : AppBar(
              actions: [
                IconButton(
                  icon: const Icon(Icons.settings),
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute(
                        builder: (_) => const SettingsScreen()),
                  ),
                ),
              ],
            ),
      body: IndexedStack(index: _index, children: _screens),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: const [
          NavigationDestination(
              icon: Icon(Icons.smart_toy), label: 'AI'),
          NavigationDestination(
              icon: Icon(Icons.movie), label: 'Media'),
          NavigationDestination(
              icon: Icon(Icons.download), label: 'Downloads'),
          NavigationDestination(
              icon: Icon(Icons.notifications), label: 'Events'),
        ],
      ),
    );
  }
}
