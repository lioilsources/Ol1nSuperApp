import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import '../../core/api_client.dart';

class ChatMessage {
  final String role;
  final String content;
  const ChatMessage({required this.role, required this.content});
  Map<String, dynamic> toJson() => {'role': role, 'content': content};
}

class AiProvider extends ChangeNotifier {
  List<String> models = [];
  String selectedModel = '';
  String? conversationId;
  List<ChatMessage> messages = [];
  bool isStreaming = false;
  String? error;
  String streamingBuffer = '';

  Future<void> loadModels() async {
    try {
      final resp = await ApiClient.instance.get('/api/ai/models');
      final data = resp.data as Map<String, dynamic>;
      final list = data['models'] as List? ?? [];
      models = list
          .map((m) => (m as Map<String, dynamic>)['name'] as String)
          .toList();
      if (models.isNotEmpty && selectedModel.isEmpty) {
        selectedModel = models.first;
      }
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  Future<void> newConversation() async {
    try {
      final resp = await ApiClient.instance.post('/api/ai/conversations/new');
      conversationId = (resp.data as Map<String, dynamic>)['conversation_id'] as String;
      messages = [];
      streamingBuffer = '';
      notifyListeners();
    } catch (e) {
      error = e.toString();
      notifyListeners();
    }
  }

  void clearError() {
    error = null;
    notifyListeners();
  }

  void setModel(String model) {
    selectedModel = model;
    notifyListeners();
  }

  Future<void> sendMessage(String text) async {
    if (isStreaming || text.trim().isEmpty) return;

    messages = [...messages, ChatMessage(role: 'user', content: text)];
    streamingBuffer = '';
    isStreaming = true;
    error = null;
    notifyListeners();

    try {
      final resp = await ApiClient.instance.post(
        '/api/ai/chat',
        data: {
          'model': selectedModel,
          'messages': messages.map((m) => m.toJson()).toList(),
          if (conversationId != null) 'conversation_id': conversationId,
        },
        options: Options(responseType: ResponseType.stream),
      );

      final stream = resp.data.stream as Stream<List<int>>;
      await for (final chunk in stream
          .transform(utf8.decoder)
          .transform(const LineSplitter())) {
        if (chunk.startsWith('data: ') && chunk != 'data: [DONE]') {
          final jsonStr = chunk.substring(6);
          try {
            final json = jsonDecode(jsonStr) as Map<String, dynamic>;
            final content =
                (json['message'] as Map<String, dynamic>?)?['content'] as String? ?? '';
            streamingBuffer += content;
            notifyListeners();
          } catch (_) {}
        }
      }

      messages = [
        ...messages,
        ChatMessage(role: 'assistant', content: streamingBuffer),
      ];
      streamingBuffer = '';
    } catch (e) {
      error = e.toString();
    } finally {
      isStreaming = false;
      notifyListeners();
    }
  }
}
