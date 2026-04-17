import 'package:flutter/material.dart';
import 'series_search.dart';
import 'movie_search.dart';

class MediaScreen extends StatelessWidget {
  const MediaScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Media'),
          bottom: const TabBar(
            tabs: [
              Tab(icon: Icon(Icons.tv), text: 'Series'),
              Tab(icon: Icon(Icons.movie), text: 'Movies'),
            ],
          ),
        ),
        body: const TabBarView(
          children: [SeriesSearch(), MovieSearch()],
        ),
      ),
    );
  }
}
