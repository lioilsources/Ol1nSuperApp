import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';
import 'config.dart';

class WebSocketService {
  WebSocketChannel? _channel;
  final _controller = StreamController<Map<String, dynamic>>.broadcast();

  Stream<Map<String, dynamic>> get events => _controller.stream;

  void connect() {
    final wsUrl = AppConfig.baseURL
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://');
    _channel = WebSocketChannel.connect(Uri.parse('$wsUrl/ws/events'));
    _channel!.stream.listen(
      (data) {
        try {
          _controller.add(jsonDecode(data as String) as Map<String, dynamic>);
        } catch (_) {}
      },
      onDone: _reconnect,
      onError: (_) => _reconnect(),
    );
  }

  void _reconnect() {
    Future.delayed(const Duration(seconds: 5), connect);
  }

  void dispose() {
    _channel?.sink.close();
    _controller.close();
  }
}
