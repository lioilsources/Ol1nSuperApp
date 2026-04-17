import 'package:flutter/material.dart';

final appTheme = ThemeData(
  useMaterial3: true,
  brightness: Brightness.dark,
  colorScheme: ColorScheme.fromSeed(
    seedColor: const Color(0xFF6750A4),
    brightness: Brightness.dark,
  ),
  navigationBarTheme: const NavigationBarThemeData(
    indicatorColor: Color(0xFF6750A4),
  ),
);
