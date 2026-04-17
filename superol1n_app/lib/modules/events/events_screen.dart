import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'events_provider.dart';

class EventsScreen extends StatelessWidget {
  const EventsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Events'),
        actions: [
          IconButton(
            icon: const Icon(Icons.clear_all),
            onPressed: () => context.read<EventsProvider>().clearEvents(),
          ),
        ],
      ),
      body: Consumer<EventsProvider>(
        builder: (context, provider, _) {
          if (provider.events.isEmpty) {
            return const Center(child: Text('No events yet'));
          }
          return ListView.builder(
            itemCount: provider.events.length,
            itemBuilder: (context, i) {
              final e = provider.events[i];
              return ListTile(
                leading: _iconForType(e.type),
                title: Text(e.type,
                    style: const TextStyle(fontFamily: 'monospace')),
                subtitle: Text(e.time),
              );
            },
          );
        },
      ),
    );
  }

  Widget _iconForType(String type) {
    if (type.startsWith('sonarr')) return const Icon(Icons.tv);
    if (type.startsWith('radarr')) return const Icon(Icons.movie);
    return const Icon(Icons.notifications_none);
  }
}
