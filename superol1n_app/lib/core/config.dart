import 'package:shared_preferences/shared_preferences.dart';

class AppConfig {
  static const _keyBaseURL = 'base_url';
  static const _keyLANKey = 'lan_key';
  static const defaultBaseURL = 'https://superol1n.ol1n.com';

  static String baseURL = defaultBaseURL;
  static String lanKey = '';

  static Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    baseURL = prefs.getString(_keyBaseURL) ?? defaultBaseURL;
    lanKey = prefs.getString(_keyLANKey) ?? '';
  }

  static Future<void> save({String? url, String? key}) async {
    final prefs = await SharedPreferences.getInstance();
    if (url != null) {
      baseURL = url;
      await prefs.setString(_keyBaseURL, url);
    }
    if (key != null) {
      lanKey = key;
      await prefs.setString(_keyLANKey, key);
    }
  }
}
