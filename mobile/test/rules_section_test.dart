// Checks that organized rules use commissioner-controlled settings from the API.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mosaic_fantasy/data/models.dart';
import 'package:mosaic_fantasy/screens/rules_section.dart';

void main() {
  testWidgets('shows general topics and current sport limits', (tester) async {
    final dynasty = Dynasty.fromJson({
      'name': 'Example',
      'franchises': [],
      'settings': {
        'overall_title': {'enabled': false},
      },
      'leagues': [
        {
          'name': 'Hockey',
          'competition': 'nhl',
          'settings': {
            'roster': {'main': 19, 'reserve': 6},
            'format': {'type': 'total_points'},
          },
        },
      ],
    });
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(child: RulesSection(dynasty: dynasty)),
        ),
      ),
    );
    await tester.tap(find.text('Rules for every league'));
    await tester.pumpAndSettle();
    expect(find.text('The basics'), findsOneWidget);
    await tester.ensureVisible(find.text('Hockey'));
    await tester.tap(find.text('Hockey'));
    await tester.pumpAndSettle();
    expect(find.text('Main roster'), findsOneWidget);
    expect(find.text('19'), findsOneWidget);
  });
}
