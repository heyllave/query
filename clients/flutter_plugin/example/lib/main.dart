import 'package:flutter/material.dart';
import 'package:trazo_query_flutter/trazo_query_flutter.dart';

void main() => runApp(const ExampleApp());

class ExampleApp extends StatelessWidget {
  const ExampleApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      title: 'trazo_query',
      home: HomePage(),
    );
  }
}

class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  static const _fields = [
    FieldConfig(name: 'state', type: FieldValueType.text, allowedOps: ['=']),
    FieldConfig(
      name: 'total',
      type: FieldValueType.decimal,
      allowedOps: ['>', '>=', '<', '<='],
    ),
  ];

  String _status = 'loading native engine…';

  @override
  void initState() {
    super.initState();
    _run();
  }

  Future<void> _run() async {
    try {
      final q = await loadTrazoQuery();
      final m = await q.match(
        'state=draft AND total>50000',
        _fields,
        {'state': 'draft', 'total': 60000},
      );
      setState(() => _status = m.ok
          ? 'matched: ${m.matched}'
          : 'error: ${m.error}');
    } catch (e) {
      setState(() => _status = 'failed to load engine: $e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('trazo_query')),
      body: Center(child: Text(_status)),
    );
  }
}
