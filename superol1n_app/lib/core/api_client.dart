import 'package:dio/dio.dart';
import 'config.dart';

class ApiClient {
  static Dio? _dio;

  static Dio get instance {
    _dio ??= _create();
    return _dio!;
  }

  static void reset() {
    _dio = null;
  }

  static Dio _create() {
    final dio = Dio(BaseOptions(
      baseUrl: AppConfig.baseURL,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 60),
    ));

    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        if (AppConfig.lanKey.isNotEmpty) {
          options.headers['X-LAN-Key'] = AppConfig.lanKey;
        }
        return handler.next(options);
      },
    ));

    return dio;
  }
}
