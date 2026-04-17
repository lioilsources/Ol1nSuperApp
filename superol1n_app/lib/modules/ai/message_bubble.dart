import 'package:flutter/material.dart';
import 'package:flutter_markdown/flutter_markdown.dart';

class MessageBubble extends StatelessWidget {
  final String role;
  final String content;
  final bool isStreaming;

  const MessageBubble({
    super.key,
    required this.role,
    required this.content,
    this.isStreaming = false,
  });

  @override
  Widget build(BuildContext context) {
    final isUser = role == 'user';
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.of(context).size.width * 0.8,
        ),
        margin: const EdgeInsets.symmetric(vertical: 4, horizontal: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: isUser
              ? Theme.of(context).colorScheme.primary
              : Theme.of(context).colorScheme.surfaceContainerHigh,
          borderRadius: BorderRadius.circular(12),
        ),
        child: isUser
            ? Text(content, style: TextStyle(
                color: Theme.of(context).colorScheme.onPrimary))
            : MarkdownBody(
                data: content + (isStreaming ? ' ▊' : ''),
                styleSheet: MarkdownStyleSheet.fromTheme(Theme.of(context)),
              ),
      ),
    );
  }
}
