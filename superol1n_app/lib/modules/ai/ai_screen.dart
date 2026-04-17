import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'ai_provider.dart';
import 'message_bubble.dart';
import 'message_input.dart';
import 'model_picker.dart';

class AiScreen extends StatefulWidget {
  const AiScreen({super.key});

  @override
  State<AiScreen> createState() => _AiScreenState();
}

class _AiScreenState extends State<AiScreen> {
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    final provider = context.read<AiProvider>();
    provider.loadModels();
    if (provider.conversationId == null) {
      provider.newConversation();
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOut,
        );
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('AI Chat'),
        actions: [
          const ModelPicker(),
          IconButton(
            icon: const Icon(Icons.add_comment),
            tooltip: 'New conversation',
            onPressed: () => context.read<AiProvider>().newConversation(),
          ),
        ],
      ),
      body: Consumer<AiProvider>(
        builder: (context, provider, _) {
          _scrollToBottom();
          return Column(
            children: [
              if (provider.error != null)
                MaterialBanner(
                  content: Text(provider.error!),
                  actions: [
                    TextButton(
                      onPressed: () => provider.clearError(),
                      child: const Text('Dismiss'),
                    ),
                  ],
                ),
              Expanded(
                child: ListView.builder(
                  controller: _scrollController,
                  itemCount: provider.messages.length +
                      (provider.isStreaming ? 1 : 0),
                  itemBuilder: (context, i) {
                    if (i < provider.messages.length) {
                      final msg = provider.messages[i];
                      return MessageBubble(
                          role: msg.role, content: msg.content);
                    }
                    return MessageBubble(
                      role: 'assistant',
                      content: provider.streamingBuffer,
                      isStreaming: true,
                    );
                  },
                ),
              ),
              const MessageInput(),
            ],
          );
        },
      ),
    );
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }
}
