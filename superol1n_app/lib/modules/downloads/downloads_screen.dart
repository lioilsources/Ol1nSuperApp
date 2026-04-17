import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'downloads_provider.dart';

class DownloadsScreen extends StatefulWidget {
  const DownloadsScreen({super.key});

  @override
  State<DownloadsScreen> createState() => _DownloadsScreenState();
}

class _DownloadsScreenState extends State<DownloadsScreen> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    context.read<DownloadsProvider>().loadQueue();
    _timer = Timer.periodic(const Duration(seconds: 10), (_) {
      context.read<DownloadsProvider>().loadQueue();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Downloads'),
        actions: [
          Consumer<DownloadsProvider>(
            builder: (context, provider, _) => IconButton(
              icon: Icon(provider.isPaused ? Icons.play_arrow : Icons.pause),
              tooltip: provider.isPaused ? 'Resume' : 'Pause',
              onPressed:
                  provider.isPaused ? provider.resume : provider.pause,
            ),
          ),
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => context.read<DownloadsProvider>().loadQueue(),
          ),
        ],
      ),
      body: Consumer<DownloadsProvider>(
        builder: (context, provider, _) {
          if (provider.loading && provider.queue.isEmpty) {
            return const Center(child: CircularProgressIndicator());
          }
          if (provider.queue.isEmpty) {
            return const Center(child: Text('Queue is empty'));
          }
          return ListView.builder(
            itemCount: provider.queue.length,
            itemBuilder: (context, i) {
              final item = provider.queue[i];
              return ListTile(
                title: Text(item.filename,
                    maxLines: 2, overflow: TextOverflow.ellipsis),
                subtitle: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    LinearProgressIndicator(value: item.percentage / 100),
                    Text('${item.percentage.toStringAsFixed(1)}%  '
                        '${item.size}  ETA: ${item.timeLeft}  [${item.status}]'),
                  ],
                ),
                trailing: IconButton(
                  icon: const Icon(Icons.delete_outline),
                  onPressed: () => provider.deleteItem(item.id),
                ),
              );
            },
          );
        },
      ),
    );
  }
}
