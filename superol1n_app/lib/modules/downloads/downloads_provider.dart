import 'package:flutter/foundation.dart';
import '../../core/api_client.dart';

class DownloadItem {
  final String id;
  final String filename;
  final double percentage;
  final String status;
  final String size;
  final String timeLeft;

  const DownloadItem({
    required this.id,
    required this.filename,
    required this.percentage,
    required this.status,
    required this.size,
    required this.timeLeft,
  });
}

class DownloadsProvider extends ChangeNotifier {
  List<DownloadItem> queue = [];
  bool loading = false;
  bool isPaused = false;
  String? error;

  Future<void> loadQueue() async {
    loading = true;
    notifyListeners();
    try {
      final resp = await ApiClient.instance.get('/api/sabnzbd/queue');
      final data = resp.data as Map<String, dynamic>;
      final slots = (data['queue'] as Map<String, dynamic>?)?['slots'] as List? ?? [];
      isPaused = (data['queue'] as Map<String, dynamic>?)?['paused'] as bool? ?? false;
      queue = slots.map((s) {
        final m = s as Map<String, dynamic>;
        return DownloadItem(
          id: m['nzo_id'] as String? ?? '',
          filename: m['filename'] as String? ?? '',
          percentage: double.tryParse(m['percentage']?.toString() ?? '0') ?? 0,
          status: m['status'] as String? ?? '',
          size: m['size'] as String? ?? '',
          timeLeft: m['timeleft'] as String? ?? '',
        );
      }).toList();
    } catch (e) {
      error = e.toString();
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  Future<void> pause() async {
    try {
      await ApiClient.instance.post('/api/sabnzbd/pause');
      isPaused = true;
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  Future<void> resume() async {
    try {
      await ApiClient.instance.post('/api/sabnzbd/resume');
      isPaused = false;
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  Future<void> deleteItem(String id) async {
    try {
      await ApiClient.instance.delete('/api/sabnzbd/queue/$id');
      queue = queue.where((d) => d.id != id).toList();
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }
}
