import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'media_provider.dart';

class SeriesSearch extends StatefulWidget {
  const SeriesSearch({super.key});

  @override
  State<SeriesSearch> createState() => _SeriesSearchState();
}

class _SeriesSearchState extends State<SeriesSearch> {
  final _controller = TextEditingController();

  @override
  void initState() {
    super.initState();
    context.read<MediaProvider>().loadSeries();
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
            ? provider.series
            : provider.seriesSearchResults;
        return Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(8),
              child: TextField(
                controller: _controller,
                decoration: const InputDecoration(
                  hintText: 'Search series...',
                  prefixIcon: Icon(Icons.search),
                  border: OutlineInputBorder(),
                ),
                onChanged: provider.searchSeries,
              ),
            ),
            Expanded(
              child: provider.loadingSeries
                  ? const Center(child: CircularProgressIndicator())
                  : ListView.builder(
                      itemCount: items.length,
                      itemBuilder: (context, i) => _SeriesCard(item: items[i]),
                    ),
            ),
          ],
        );
      },
    );
  }
}

class _SeriesCard extends StatelessWidget {
  final MediaItem item;
  const _SeriesCard({required this.item});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: item.posterUrl != null
          ? CachedNetworkImage(
              imageUrl: item.posterUrl!,
              width: 40,
              height: 60,
              fit: BoxFit.cover,
              placeholder: (c, u) =>
                  const SizedBox(width: 40, height: 60, child: Icon(Icons.tv)),
              errorWidget: (c, u, e) =>
                  const SizedBox(width: 40, height: 60, child: Icon(Icons.tv)),
            )
          : const Icon(Icons.tv),
      title: Text(item.title),
      subtitle: item.overview != null
          ? Text(item.overview!, maxLines: 2, overflow: TextOverflow.ellipsis)
          : null,
    );
  }
}
