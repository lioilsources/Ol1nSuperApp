import 'package:flutter/foundation.dart';
import '../../core/websocket_service.dart';

class EventItem {
  final String type;
  final String time;
  final Map<String, dynamic> payload;
  const EventItem(
      {required this.type, required this.time, required this.payload});
}

class EventsProvider extends ChangeNotifier {
  final WebSocketService _ws;
  final List<EventItem> events = [];

  EventsProvider(this._ws) {
    _ws.connect();
    _ws.events.listen((data) {
      events.insert(
          0,
          EventItem(
            type: data['type'] as String? ?? 'unknown',
            time: data['time'] as String? ?? '',
            payload: data['payload'] as Map<String, dynamic>? ?? {},
          ));
      if (events.length > 200) events.removeLast();
      notifyListeners();
    });
  }

  void clearEvents() {
    events.clear();
    notifyListeners();
  }

  @override
  void dispose() {
    _ws.dispose();
    super.dispose();
  }
}
