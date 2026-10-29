// Displays a franchise image or an accessible monogram when no usable image exists.

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';

class ProfileAvatar extends StatelessWidget {
  const ProfileAvatar({
    super.key,
    required this.name,
    this.imageUrl = '',
    this.size = 48,
  });

  final String name;
  final String imageUrl;
  final double size;

  String get initials {
    final words = name
        .trim()
        .split(RegExp(r'\s+'))
        .where((part) => part.isNotEmpty)
        .toList();
    if (words.isEmpty) return 'MF';
    return words.take(2).map((part) => part[0].toUpperCase()).join();
  }

  @override
  Widget build(BuildContext context) {
    final fallback = Center(
      child: Text(
        initials,
        style: TextStyle(
          fontSize: size * .34,
          fontWeight: FontWeight.w800,
          color: Colors.white,
        ),
      ),
    );
    Widget image = fallback;
    try {
      if (imageUrl.startsWith('data:image/')) {
        final bytes = Uint8List.fromList(
          base64Decode(imageUrl.split(',').last),
        );
        image = Image.memory(
          bytes,
          fit: BoxFit.cover,
          width: size,
          height: size,
          errorBuilder: (_, _, _) => fallback,
        );
      } else if (imageUrl.startsWith('https://') ||
          imageUrl.startsWith('http://')) {
        image = Image.network(
          imageUrl,
          fit: BoxFit.cover,
          width: size,
          height: size,
          errorBuilder: (_, _, _) => fallback,
        );
      }
    } catch (_) {
      image = fallback;
    }
    return Semantics(
      label: '$name image',
      child: Container(
        width: size,
        height: size,
        clipBehavior: Clip.antiAlias,
        decoration: BoxDecoration(
          color: const Color(0xffab355e),
          borderRadius: BorderRadius.circular(size * .28),
        ),
        child: image,
      ),
    );
  }
}
