import 'package:flutter/foundation.dart';
import '../../core/api_client.dart';

class MediaItem {
  final int id;
  final String title;
  final String? overview;
  final String? posterUrl;

  const MediaItem(
      {required this.id,
      required this.title,
      this.overview,
      this.posterUrl});
}

class MediaProvider extends ChangeNotifier {
  List<MediaItem> series = [];
  List<MediaItem> movies = [];
  List<MediaItem> seriesSearchResults = [];
  List<MediaItem> movieSearchResults = [];
  bool loadingSeries = false;
  bool loadingMovies = false;
  String? error;

  Future<void> loadSeries() async {
    loadingSeries = true;
    notifyListeners();
    try {
      final resp = await ApiClient.instance.get('/api/sonarr/series');
      final list = resp.data as List;
      series = list.map((e) => _parseSeries(e as Map<String, dynamic>)).toList();
    } catch (e) {
      error = e.toString();
    } finally {
      loadingSeries = false;
      notifyListeners();
    }
  }

  Future<void> loadMovies() async {
    loadingMovies = true;
    notifyListeners();
    try {
      final resp = await ApiClient.instance.get('/api/radarr/movie');
      final list = resp.data as List;
      movies = list.map((e) => _parseMovie(e as Map<String, dynamic>)).toList();
    } catch (e) {
      error = e.toString();
    } finally {
      loadingMovies = false;
      notifyListeners();
    }
  }

  Future<void> searchSeries(String query) async {
    if (query.isEmpty) {
      seriesSearchResults = [];
      notifyListeners();
      return;
    }
    try {
      final resp = await ApiClient.instance
          .get('/api/sonarr/series/lookup', queryParameters: {'q': query});
      final list = resp.data as List;
      seriesSearchResults =
          list.map((e) => _parseSeries(e as Map<String, dynamic>)).toList();
    } catch (e) {
      error = e.toString();
    }
    notifyListeners();
  }

  Future<void> searchMovies(String query) async {
    if (query.isEmpty) {
      movieSearchResults = [];
      notifyListeners();
      return;
    }
    try {
      final resp = await ApiClient.instance
          .get('/api/radarr/movie/lookup', queryParameters: {'q': query});
      final list = resp.data as List;
      movieSearchResults =
          list.map((e) => _parseMovie(e as Map<String, dynamic>)).toList();
    } catch (e) {
      error = e.toString();
    }
    notifyListeners();
  }

  MediaItem _parseSeries(Map<String, dynamic> e) {
    final images = e['images'] as List? ?? [];
    final poster = images.firstWhere(
        (i) => (i as Map)['coverType'] == 'poster',
        orElse: () => null);
    return MediaItem(
      id: e['id'] as int? ?? 0,
      title: e['title'] as String? ?? '',
      overview: e['overview'] as String?,
      posterUrl: poster != null
          ? (poster as Map<String, dynamic>)['remoteUrl'] as String?
          : null,
    );
  }

  MediaItem _parseMovie(Map<String, dynamic> e) {
    final images = e['images'] as List? ?? [];
    final poster = images.firstWhere(
        (i) => (i as Map)['coverType'] == 'poster',
        orElse: () => null);
    return MediaItem(
      id: e['id'] as int? ?? 0,
      title: e['title'] as String? ?? '',
      overview: e['overview'] as String?,
      posterUrl: poster != null
          ? (poster as Map<String, dynamic>)['remoteUrl'] as String?
          : null,
    );
  }
}
