import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'ai_provider.dart';

class ModelPicker extends StatelessWidget {
  const ModelPicker({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<AiProvider>(
      builder: (context, provider, _) {
        if (provider.models.isEmpty) return const SizedBox.shrink();
        return DropdownButton<String>(
          value: provider.selectedModel,
          onChanged: (v) {
            if (v != null) provider.setModel(v);
          },
          items: provider.models
              .map((m) => DropdownMenuItem(value: m, child: Text(m)))
              .toList(),
        );
      },
    );
  }
}
