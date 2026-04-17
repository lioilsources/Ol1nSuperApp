import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'media_provider.dart';

class MovieSearch extends StatefulWidget {
  const MovieSearch({super.key});

  @override
  State<MovieSearch> createState() => _MovieSearchState();
}

class _MovieSearchState extends State<MovieSearch> {
  final _controller = TextEditingController();

  @override
  void initState() {
    super.initState();
    context.read<MediaProvider>().loadMovies();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Consumer<MediaProvider>(
      builder: (context, provider, _) {
        final items = _controller.text.isEmpty
            ? provider.movies
            : provider.movieSearchResults;
        return Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(8),
              child: TextField(
                controller: _controller,
                decoration: const InputDecoration(
                  hintText: 'Search movies...',
                  prefixIcon: Icon(Icons.search),
                  border: OutlineInputBorder(),
                ),
                onChanged: provider.searchMovies,
              ),
            ),
            Expanded(
              child: provider.loadingMovies
                  ? const Center(child: CircularProgressIndicator())
                  : ListView.builder(
                      itemCount: items.length,
                      itemBuilder: (context, i) => _MovieCard(item: items[i]),
                    ),
            ),
          ],
        );
      },
    );
  }
}

class _MovieCard extends StatelessWidget {
  final MediaItem item;
  const _MovieCard({required this.item});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: item.posterUrl != null
          ? CachedNetworkImage(
              imageUrl: item.posterUrl!,
              width: 40,
              height: 60,
              fit: BoxFit.cover,
              placeholder: (c, u) => const SizedBox(
                  width: 40, height: 60, child: Icon(Icons.movie)),
              errorWidget: (c, u, e) => const SizedBox(
                  width: 40, height: 60, child: Icon(Icons.movie)),
            )
          : const Icon(Icons.movie),
      title: Text(item.title),
      subtitle: item.overview != null
          ? Text(item.overview!, maxLines: 2, overflow: TextOverflow.ellipsis)
          : null,
    );
  }
}
